package core

func (c *irCompiler) tryInlineCall(fnSlot int, expr *CallExpr, isLast bool) bool {
	_ = fnSlot
	if irInlineDisabled() {
		return false
	}
	fnExpr := findFnExprForBinding(expr.callable)
	if fnExpr == nil || len(fnExpr.arities) != 1 || fnExpr.variadic != nil {
		return false
	}
	arity := fnExpr.arities[0]
	if !irInlineForce() {
		inlineOK := false
		for _, b := range arity.body {
			if exprHasTextLiteralOrStr(b) {
				inlineOK = true
				break
			}
			if exprIsStraightLine(b) {
				if exprHasCollectionOp(b) && exprCount(b) <= 16 {
					inlineOK = true
					break
				}
				// Inline pure arithmetic helpers (≤32 exprs) only when the
				// caller loop has no collection ops.
				if exprIsPureArithmetic(b) && exprCount(b) <= 32 && !c.hasCollectionOps {
					inlineOK = true
					break
				}
			}
		}
		if !inlineOK {
			return false
		}
	}
	if len(arity.args) != len(expr.args) || len(arity.body) != 1 {
		return false
	}
	fnFrame := guessLoopFrame(arity.body)
	if fnFrame < 0 {
		return false
	}
	// Use a synthetic frame to avoid collision with the caller's loop frame.
	// The fn's parameters may share the same (frame, index) as the caller's
	// loop bindings. By remapping to a unique frame, inline temps don't
	// overwrite caller slots.
	inlineFrame := fnFrame + 1000
	for _, arg := range expr.args {
		if !c.compileExpr(arg, false) {
			return false
		}
	}
	baseSlot := c.numSlots
	oldBindings := make(map[bindingKey]int, len(arity.args))
	oldPresent := make(map[bindingKey]bool, len(arity.args))
	for i := len(arity.args) - 1; i >= 0; i-- {
		slot := baseSlot + i
		key := bindingKey{frame: inlineFrame, index: i}
		if old, ok := c.bindingMap[key]; ok {
			oldBindings[key] = old
			oldPresent[key] = true
		}
		c.bindingMap[key] = slot
		c.emitWithOperand(irStoreSlot, slot)
	}
	// Also remap the original fnFrame bindings so body references resolve
	origKeys := make([]bindingKey, len(arity.args))
	origOld := make(map[bindingKey]int)
	origPresent := make(map[bindingKey]bool)
	for i := range arity.args {
		origKey := bindingKey{frame: fnFrame, index: i}
		origKeys[i] = origKey
		if old, ok := c.bindingMap[origKey]; ok {
			origOld[origKey] = old
			origPresent[origKey] = true
		}
		c.bindingMap[origKey] = baseSlot + i
	}
	c.numSlots = baseSlot + len(arity.args)
	// The inlined body may contain let/or expansions at frames that
	// collide with the caller's loop bindings. To avoid findLetFrame
	// skipping those frames ("already known"), temporarily clear
	// caller bindings at the inlined body's internal let frames.
	inlineLetFrames := collectLetFrames(arity.body[0], fnFrame)
	savedInlineFrames := make(map[bindingKey]int)
	for k, v := range c.bindingMap {
		for _, lf := range inlineLetFrames {
			if k.frame == lf {
				savedInlineFrames[k] = v
			}
		}
	}
	for k := range savedInlineFrames {
		delete(c.bindingMap, k)
	}
	ok := c.compileExpr(arity.body[0], isLast)
	for k, v := range savedInlineFrames {
		c.bindingMap[k] = v
	}
	for i := range arity.args {
		key := bindingKey{frame: inlineFrame, index: i}
		if oldPresent[key] {
			c.bindingMap[key] = oldBindings[key]
		} else {
			delete(c.bindingMap, key)
		}
		origKey := origKeys[i]
		if origPresent[origKey] {
			c.bindingMap[origKey] = origOld[origKey]
		} else {
			delete(c.bindingMap, origKey)
		}
	}
	return ok
}

// findFnExprForBinding tries to find the FnExpr for a callable binding.
func findFnExprForBinding(callable Expr) *FnExpr {
	bindExpr, ok := callable.(*BindingExpr)
	if !ok {
		return nil
	}
	if bindExpr.binding.value == nil {
		return nil
	}
	if fnExpr, ok := bindExpr.binding.value.(*FnExpr); ok {
		return fnExpr
	}
	return nil
}

func (c *irCompiler) compileCall(expr *CallExpr, isLast bool) bool {
	// Check if callable is a binding (local/captured function)
	if bindExpr, ok := expr.callable.(*BindingExpr); ok {
		// Check for self-recursive call
		if c.hasSelf && bindExpr.binding.frame < c.loopFrame && len(expr.args) == c.selfNArgs {
			for _, arg := range expr.args {
				if !c.compileExpr(arg, false) {
					return false
				}
			}
			c.emitWithOperand(irCallSelf, len(expr.args))
			if isLast {
				c.emit(irReturn)
			}
			return true
		}

		key := bindingKey{frame: bindExpr.binding.frame, index: bindExpr.binding.index}
		slot, ok := c.bindingMap[key]
		if !ok {
			if bindExpr.binding.frame < c.loopFrame {
				slot = c.numSlots
				c.bindingMap[key] = slot
				c.captureKeys = append(c.captureKeys, key)
				c.numSlots++
			} else {
				return c.reject("callable binding frame %d index %d is not capturable from loop frame %d", bindExpr.binding.frame, bindExpr.binding.index, c.loopFrame)
			}
		}

		// Try to inline the function call
		if c.tryInlineCall(slot, expr, isLast) {
			return true
		}

		for _, arg := range expr.args {
			if !c.compileExpr(arg, false) {
				return false
			}
		}
		c.code = append(c.code, irCallSlot,
			byte(slot>>8), byte(slot),
			byte(len(expr.args)>>8), byte(len(expr.args)))
		if isLast {
			c.emit(irReturn)
		}
		return true
	}

	vref, ok := expr.callable.(*VarRefExpr)
	if !ok {
		return c.reject("unsupported callable expression type %T", expr.callable)
	}
	procName := ""
	switch v := vref.vr.Value.(type) {
	case Proc:
		procName = v.Name
	case *Fn:
		procName = coreVarToProcName(vref.vr)
	}
	if procName == "" {
		return c.reject("unsupported callable var %s", vref.vr.name.ToString(false))
	}

	switch procName {
	case "procAdd":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irAdd)
	case "procSubtract":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irSub)
	case "procMultiply":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irMul)
	case "procRem":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irRem)
	case "procDivide":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irDiv)
	case "procInc":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irInc)
	case "procDec":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irDec)
	case "procLt":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irLt)
	case "procGte":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irGte)
	case "procGt":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irGt)
	case "procLte":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irLte)
	case "procEq":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irEq)
	case "procInt":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg", procName)
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irIntCast)
	case "procSubs":
		if len(expr.args) < 2 || len(expr.args) > 3 {
			return c.reject("%s expects 2-3 args", procName)
		}
		for _, a := range expr.args {
			if !c.compileExpr(a, false) {
				return false
			}
		}
		// Encode arg count in the opcode operand
		c.emitWithOperand(irSubs, len(expr.args))
	case "procIsZero":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irIsZero)
	case "procGet":
		c.hasCollectionOps = true
		if len(expr.args) == 2 {
			if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
				return false
			}
			c.emit(irGet)
		} else if len(expr.args) == 3 {
			if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) || !c.compileExpr(expr.args[2], false) {
				return false
			}
			c.emit(irGet3)
		} else {
			return c.reject("%s expects 2 or 3 args, got %d", procName, len(expr.args))
		}
	case "procAssoc":
		c.hasCollectionOps = true
		if len(expr.args) != 3 {
			return c.reject("%s expects 3 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) || !c.compileExpr(expr.args[2], false) {
			return false
		}
		c.emit(irAssoc)
	case "procNth":
		c.hasCollectionOps = true
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if s, ok := c.constantASCIIString(expr.args[0]); ok {
			if !c.compileExpr(expr.args[1], false) {
				return false
			}
			idx := c.addConstant(String{S: s})
			c.emitWithOperand(irNthStringASCII, idx)
		} else {
			if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
				return false
			}
			c.emit(irNth)
		}
	case "procConj":
		c.hasCollectionOps = true
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irConj)
	case "procSqrt":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irSqrt)
	case "procFirst":
		c.hasCollectionOps = true
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irFirst)
	case "procCursorChar":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irCursorChar)
	case "procCursorNext":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irCursorNext)
	case "procCursorDone":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irCursorDone)
	case "procStr":
		if len(expr.args) == 1 {
			if !c.compileExpr(expr.args[0], false) {
				return false
			}
			c.emit(irStr1)
		} else if len(expr.args) == 2 {
			if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
				return false
			}
			c.emit(irStr2)
		} else {
			return c.reject("%s expects 1 or 2 args, got %d", procName, len(expr.args))
		}
	case "procCount":
		c.hasCollectionOps = true
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if n, ok := c.constantCount(expr.args[0]); ok {
			idx := c.addConstant(Int{I: n})
			c.emitWithOperand(irLiteral, idx)
		} else {
			if !c.compileExpr(expr.args[0], false) {
				return false
			}
			c.emit(irCount)
		}
	case "procApply":
		if len(expr.args) != 2 {
			return c.reject("apply expects 2 args (fn + args), got %d", len(expr.args))
		}
		// Compile fn and args-seq onto stack, then irApply
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		if !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irApply)
	case "procThrow":
		if len(expr.args) != 1 {
			return c.reject("throw expects 1 arg, got %d", len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irThrow)
	default:
		return c.reject("unsupported core proc for IR: %s", procName)
	}
	if isLast {
		c.emit(irReturn)
	}
	return true
}

// coreVarToProcName maps well-known core Vars to internal proc names.
func coreVarToProcName(vr *Var) string {
	if vr.ns == nil || vr.ns != GLOBAL_ENV.CoreNamespace {
		return ""
	}
	switch vr.name.ToString(false) {
	case "+":
		return "procAdd"
	case "-":
		return "procSubtract"
	case "*":
		return "procMultiply"
	case "rem":
		return "procRem"
	case "inc":
		return "procInc"
	case "dec":
		return "procDec"
	case "<":
		return "procLt"
	case "=":
		return "procEq"
	case "zero?":
		return "procIsZero"
	case "/":
		return "procDivide"
	case "get":
		return "procGet"
	case "assoc":
		return "procAssoc"
	case "conj":
		return "procConj"
	case "sqrt":
		return "procSqrt"
	case "first":
		return "procFirst"
	case "str":
		return "procStr"
	case "count":
		return "procCount"
	case "nth":
		return "procNth"
	case "int":
		return "procInt"
	case "subs":
		return "procSubs"
	default:
		return ""
	}
}

// collectLetFrames finds all frames used by LetExpr nodes inside an expression
// that are deeper than fnFrame (i.e., internal to the inlined fn body).
func collectLetFrames(expr Expr, fnFrame int) []int {
	var frames []int
	seen := map[int]bool{}
	var scan func(e Expr)
	scan = func(e Expr) {
		switch x := e.(type) {
		case *LetExpr:
			// Check what frame this let's bindings use
			for _, b := range x.body {
				scanBindings(b, len(x.values), fnFrame, seen, &frames)
			}
			for _, v := range x.values {
				scan(v)
			}
			for _, b := range x.body {
				scan(b)
			}
		case *IfExpr:
			scan(x.cond)
			scan(x.positive)
			scan(x.negative)
		case *CallExpr:
			scan(x.callable)
			for _, a := range x.args {
				scan(a)
			}
		}
	}
	scan(expr)
	return frames
}

func scanBindings(expr Expr, nBinds int, fnFrame int, seen map[int]bool, frames *[]int) {
	switch x := expr.(type) {
	case *BindingExpr:
		f := x.binding.frame
		if f > fnFrame && x.binding.index < nBinds && !seen[f] {
			seen[f] = true
			*frames = append(*frames, f)
		}
	case *IfExpr:
		scanBindings(x.cond, nBinds, fnFrame, seen, frames)
		scanBindings(x.positive, nBinds, fnFrame, seen, frames)
		scanBindings(x.negative, nBinds, fnFrame, seen, frames)
	case *CallExpr:
		scanBindings(x.callable, nBinds, fnFrame, seen, frames)
		for _, a := range x.args {
			scanBindings(a, nBinds, fnFrame, seen, frames)
		}
	case *LetExpr:
		for _, v := range x.values {
			scanBindings(v, nBinds, fnFrame, seen, frames)
		}
		for _, b := range x.body {
			scanBindings(b, nBinds, fnFrame, seen, frames)
		}
	}
}
