package core

import (
	"fmt"
	"os"
)

// ---------- Compiler ----------

type bindingKey struct {
	frame int
	index int
}

type irCompiler struct {
	code             []byte
	constants        []Object
	bindingMap       map[bindingKey]int
	captureKeys      []bindingKey
	captureSlots     []Object
	captureSlotIdxs  []int
	numSlots         int
	loopFrame        int
	depth            int
	hasSelf          bool
	selfSlot         int
	selfNArgs        int
	recurTargets     []recurTarget
	rejectReason     string
	hasCollectionOps bool
	fnExprs          []*FnExpr
}

type recurTarget struct {
	pc       int // bytecode offset of loop start
	baseSlot int // first slot of this loop's bindings
	nBinds   int // number of loop bindings
}

func irCompile(loop *LoopExpr) *IRProgram {
	prog, _ := irCompileExplain(loop)
	return prog
}

func irCompileExplain(loop *LoopExpr) (*IRProgram, string) {
	c := &irCompiler{
		bindingMap: make(map[bindingKey]int),
		loopFrame:  -1,
	}
	// Pre-scan loop body for collection ops to gate arithmetic helper inlining
	le := (*LetExpr)(loop)
	for _, b := range le.body {
		if exprHasCollectionOp(b) {
			c.hasCollectionOps = true
			break
		}
	}
	c.numSlots = len(loop.names)

	loopLet := (*LetExpr)(loop)
	c.loopFrame = guessLoopFrame(loopLet.body)
	if c.loopFrame < 0 {
		c.loopFrame = 1
	}
	for i := range loop.names {
		c.bindingMap[bindingKey{frame: c.loopFrame, index: i}] = i
	}

	// Push the top-level recur target (PC=0, slots 0..n-1)
	c.recurTargets = []recurTarget{{pc: 0, baseSlot: 0, nBinds: len(loop.names)}}

	for i, expr := range loopLet.body {
		if !c.compileExpr(expr, i == len(loopLet.body)-1) {
			return nil, c.reasonOr("IR compiler rejected loop body")
		}
	}
	if len(c.code) == 0 {
		return nil, "IR compiler emitted no code"
	}
	if c.code[len(c.code)-1] != irReturn && c.code[len(c.code)-1] != irJump {
		c.emit(irReturn)
	}
	// Safety limit: too many captures indicates complex nested scoping
	if len(c.captureKeys) > 12 {
		return nil, fmt.Sprintf("too many captured bindings: %d > 12", len(c.captureKeys))
	}
	// Validate: ensure no slot is assigned twice
	slotUsed := make(map[int]bool, c.numSlots)
	for _, slot := range c.bindingMap {
		if slotUsed[slot] {
			return nil, fmt.Sprintf("IR slot collision detected at slot %d", slot)
		}
		slotUsed[slot] = true
	}
	return &IRProgram{
		code:        c.code,
		constants:   c.constants,
		numSlots:    c.numSlots,
		captureKeys: c.captureKeys,
	}, ""
}

func (c *irCompiler) reject(format string, args ...interface{}) bool {
	if c.rejectReason == "" {
		c.rejectReason = fmt.Sprintf(format, args...)
	}
	return false
}

func (c *irCompiler) reasonOr(fallback string) string {
	if c.rejectReason != "" {
		return c.rejectReason
	}
	return fallback
}

// guessFnParamFrame scans a fn body for BindingExpr nodes that reference
// indices 0..nparams-1, returning the common frame. Returns -1 if ambiguous.
func guessFnParamFrame(body []Expr, nparams int) int {
	if nparams == 0 {
		return -1
	}
	// Collect all (frame, index) pairs from BindingExprs with index < nparams.
	// The fn param frame is the smallest frame where ALL indices 0..nparams-1 appear.
	frameSeen := map[int]map[int]bool{}
	var scan func(e Expr)
	scan = func(e Expr) {
		switch x := e.(type) {
		case *BindingExpr:
			if x.binding.index < nparams {
				if frameSeen[x.binding.frame] == nil {
					frameSeen[x.binding.frame] = map[int]bool{}
				}
				frameSeen[x.binding.frame][x.binding.index] = true
			}
		case *LoopExpr:
			le := (*LetExpr)(x)
			for _, v := range le.values {
				scan(v)
			}
			for _, b := range le.body {
				scan(b)
			}
		case *LetExpr:
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
		case *RecurExpr:
			for _, a := range x.args {
				scan(a)
			}
		}
	}
	for _, e := range body {
		scan(e)
	}
	// Find smallest frame with all nparams indices present
	bestFrame := -1
	for f, idxSet := range frameSeen {
		if len(idxSet) >= nparams {
			if bestFrame < 0 || f < bestFrame {
				bestFrame = f
			}
		}
	}
	return bestFrame
}

func guessLoopFrame(body []Expr) int {
	for _, expr := range body {
		if f := findRecurBindingFrame(expr); f >= 0 {
			return f
		}
	}
	for _, expr := range body {
		if f := findBindingFrame(expr); f >= 0 {
			return f
		}
	}
	return -1
}

func findRecurBindingFrame(expr Expr) int {
	switch e := expr.(type) {
	case *RecurExpr:
		for _, arg := range e.args {
			if f := findBindingFrame(arg); f >= 0 {
				return f
			}
		}
	case *IfExpr:
		if f := findRecurBindingFrame(e.positive); f >= 0 {
			return f
		}
		return findRecurBindingFrame(e.negative)
	case *LetExpr:
		for _, b := range e.body {
			if f := findRecurBindingFrame(b); f >= 0 {
				return f
			}
		}
	case *CallExpr:
		for _, arg := range e.args {
			if f := findRecurBindingFrame(arg); f >= 0 {
				return f
			}
		}
	}
	return -1
}

func findBindingFrame(expr Expr) int {
	switch e := expr.(type) {
	case *BindingExpr:
		return e.binding.frame
	case *IfExpr:
		if f := findBindingFrame(e.cond); f >= 0 {
			return f
		}
		if f := findBindingFrame(e.positive); f >= 0 {
			return f
		}
		return findBindingFrame(e.negative)
	case *CallExpr:
		for _, arg := range e.args {
			if f := findBindingFrame(arg); f >= 0 {
				return f
			}
		}
	case *RecurExpr:
		for _, arg := range e.args {
			if f := findBindingFrame(arg); f >= 0 {
				return f
			}
		}
	case *LetExpr:
		for _, v := range e.values {
			if f := findBindingFrame(v); f >= 0 {
				return f
			}
		}
	}
	return -1
}

func (c *irCompiler) emit(op byte) {
	c.code = append(c.code, op)
}

func (c *irCompiler) emitWithOperand(op byte, operand int) {
	c.code = append(c.code, op, byte(operand>>8), byte(operand))
}

func (c *irCompiler) patchJump(pos int, target int) {
	c.code[pos+1] = byte(target >> 8)
	c.code[pos+2] = byte(target)
}

func (c *irCompiler) addConstant(obj Object) int {
	for i, existing := range c.constants {
		if existing.Equals(obj) {
			return i
		}
	}
	c.constants = append(c.constants, obj)
	return len(c.constants) - 1
}

func isASCIIBytes(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func (c *irCompiler) constantASCIIString(expr Expr) (string, bool) {
	switch e := expr.(type) {
	case *LiteralExpr:
		if s, ok := e.obj.(String); ok && isASCIIBytes(s.S) {
			return s.S, true
		}
	case *BindingExpr:
		if lit, ok := e.binding.value.(*LiteralExpr); ok {
			if s, ok := lit.obj.(String); ok && isASCIIBytes(s.S) {
				return s.S, true
			}
		}
	}
	return "", false
}

func (c *irCompiler) constantCount(expr Expr) (int, bool) {
	switch e := expr.(type) {
	case *LiteralExpr:
		switch v := e.obj.(type) {
		case String:
			return v.Count(), true
		case Counted:
			return v.Count(), true
		}
	case *BindingExpr:
		// Only fold captured/outer bindings. Loop-local bindings can change via
		// recur even when their initial value is a literal.
		if e.binding.frame < c.loopFrame {
			if lit, ok := e.binding.value.(*LiteralExpr); ok {
				switch v := lit.obj.(type) {
				case String:
					return v.Count(), true
				case Counted:
					return v.Count(), true
				}
			}
		}
	}
	return 0, false
}

func (c *irCompiler) compileExpr(expr Expr, isLast bool) bool {
	switch e := expr.(type) {
	case *LiteralExpr:
		idx := c.addConstant(e.obj)
		c.emitWithOperand(irLiteral, idx)
		if isLast {
			c.emit(irReturn)
		}
		return true

	case *VectorExpr:
		// Try constant vector first (all elements are literals)
		allLiteral := true
		for _, elem := range e.v {
			if _, ok := elem.(*LiteralExpr); !ok {
				allLiteral = false
				break
			}
		}
		if allLiteral {
			arr := make([]Object, len(e.v))
			for i, elem := range e.v {
				arr[i] = elem.(*LiteralExpr).obj
			}
			idx := c.addConstant(&ArrayVector{arr: arr})
			c.emitWithOperand(irLiteral, idx)
		} else {
			// Compile each element, then emit a vector-build opcode
			for _, elem := range e.v {
				if !c.compileExpr(elem, false) {
					return false
				}
			}
			c.emitWithOperand(irBuildVec, len(e.v))
		}
		if isLast {
			c.emit(irReturn)
		}
		return true

	case *MapExpr:
		allLiteral := true
		for i := range e.keys {
			if _, ok := e.keys[i].(*LiteralExpr); !ok {
				allLiteral = false
				break
			}
			if _, ok := e.values[i].(*LiteralExpr); !ok {
				allLiteral = false
				break
			}
		}
		if !allLiteral {
			return c.reject("unsupported dynamic map literal in IR")
		}
		var obj Object
		if int64(len(e.keys)) > HASHMAP_THRESHOLD/2 {
			res := EmptyHashMap
			for i := range e.keys {
				key := e.keys[i].(*LiteralExpr).obj
				if res.containsKey(key) {
					return c.reject("duplicate key in IR map literal: %s", key.ToString(false))
				}
				res = res.Assoc(key, e.values[i].(*LiteralExpr).obj).(*HashMap)
			}
			obj = res
		} else {
			res := EmptyArrayMap()
			for i := range e.keys {
				key := e.keys[i].(*LiteralExpr).obj
				if !res.Add(key, e.values[i].(*LiteralExpr).obj) {
					return c.reject("duplicate key in IR map literal: %s", key.ToString(false))
				}
			}
			obj = res
		}
		idx := c.addConstant(obj)
		c.emitWithOperand(irLiteral, idx)
		if isLast {
			c.emit(irReturn)
		}
		return true

	case *BindingExpr:
		key := bindingKey{frame: e.binding.frame, index: e.binding.index}
		slot, ok := c.bindingMap[key]
		if !ok {
			if e.binding.frame < c.loopFrame {
				slot = c.numSlots
				c.bindingMap[key] = slot
				c.captureKeys = append(c.captureKeys, key)
				c.numSlots++
			} else {
				return c.reject("binding frame %d index %d is not in loop frame %d and cannot be captured", e.binding.frame, e.binding.index, c.loopFrame)
			}
		}
		c.emitWithOperand(irLoadSlot, slot)
		if isLast {
			c.emit(irReturn)
		}
		return true

	case *IfExpr:
		if !c.compileExpr(e.cond, false) {
			return false
		}
		jumpPos := len(c.code)
		c.emitWithOperand(irJumpIfNot, 0)
		if !c.compileExpr(e.positive, isLast) {
			return false
		}
		if !isLast {
			skipPos := len(c.code)
			c.emitWithOperand(irJump, 0)
			c.patchJump(jumpPos, len(c.code))
			if !c.compileExpr(e.negative, isLast) {
				return false
			}
			c.patchJump(skipPos, len(c.code))
		} else {
			c.patchJump(jumpPos, len(c.code))
			if !c.compileExpr(e.negative, isLast) {
				return false
			}
		}
		return true

	case *CallExpr:
		return c.compileCall(e, isLast)

	case *RecurExpr:
		if len(c.recurTargets) == 0 {
			return c.reject("recur used outside a loop target")
		}
		target := c.recurTargets[len(c.recurTargets)-1]
		for _, arg := range e.args {
			if !c.compileExpr(arg, false) {
				return false
			}
		}
		// Emit: nargs (2) + targetPC (2) [+ baseSlot (2) if nested]
		c.code = append(c.code, irRecur,
			byte(len(e.args)>>8), byte(len(e.args)),
			byte(target.pc>>8), byte(target.pc))
		if target.pc != 0 {
			// Nested loop: also emit baseSlot
			c.code = append(c.code, byte(target.baseSlot>>8), byte(target.baseSlot))
		}
		return true

	case *LetExpr:
		if c.depth > 16 {
			return c.reject("IR nesting depth exceeded for let: %d > 16", c.depth)
		}
		c.depth++
		return c.compileLetBody(e, isLast)

	case *LoopExpr:
		if c.depth > 16 {
			return c.reject("IR nesting depth exceeded for nested loop: %d > 16", c.depth)
		}
		c.depth++
		return c.compileNestedLoop(e, isLast)

	case *TryExpr:
		return c.compileTryCatch(e, isLast)

	case *FnExpr:
		// Store FnExpr index for irMakeFn opcode
		if c.fnExprs == nil {
			c.fnExprs = make([]*FnExpr, 0)
		}
		idx := len(c.fnExprs)
		c.fnExprs = append(c.fnExprs, e)
		c.emitWithOperand(irMakeFn, idx)
		if isLast {
			c.emit(irReturn)
		}
		return true

	case *DoExpr:
		for i, bodyExpr := range e.body {
			if !c.compileExpr(bodyExpr, isLast && i == len(e.body)-1) {
				return false
			}
			if i < len(e.body)-1 {
				c.emit(irPop)
			}
		}
		if len(e.body) == 0 {
			c.emitWithOperand(irLiteral, c.addConstant(NIL))
			if isLast { c.emit(irReturn) }
		}
		return true

	default:
		return c.reject("unsupported IR expression type %T", expr)
	}
}

func (c *irCompiler) compileLetBody(e *LetExpr, isLast bool) bool {
	// Detect let frame using precise binding reference analysis
	letFrame := findLetFrame(e.body, len(e.values), c.bindingMap)
	if letFrame < 0 {
		for _, bodyExpr := range e.body {
			if f := findBindingFrame(bodyExpr); f > c.loopFrame {
				letFrame = f
				break
			}
		}
	}
	if letFrame < 0 {
		letFrame = c.loopFrame + c.depth
	}
	// Save ALL existing bindings for this frame (not just the indices we'll
	// overwrite) so we can restore after the let scope exits. This prevents
	// inner let scopes from corrupting outer scope binding maps when the
	// parser assigns the same frame number to multiple scopes.
	savedBindings := make(map[bindingKey]int)
	for key, slot := range c.bindingMap {
		if key.frame == letFrame {
			savedBindings[key] = slot
		}
	}
	for i, bindExpr := range e.values {
		if !c.compileExpr(bindExpr, false) {
			return false
		}
		// Allocate the let slot after compiling the value expression: the
		// value may capture an outer binding, which grows c.numSlots. Using
		// a stale baseSlot would collide with those capture slots and make
		// otherwise valid loops non-compilable.
		slot := c.numSlots
		c.numSlots++
		c.bindingMap[bindingKey{frame: letFrame, index: i}] = slot
		c.emitWithOperand(irStoreSlot, slot)
	}
	for i, bodyExpr := range e.body {
		if !c.compileExpr(bodyExpr, isLast && i == len(e.body)-1) {
			return false
		}
	}
	// Restore outer scope bindings for this frame.
	// First, delete all current frame bindings, then restore saved ones.
	for key := range c.bindingMap {
		if key.frame == letFrame {
			delete(c.bindingMap, key)
		}
	}
	for key, slot := range savedBindings {
		c.bindingMap[key] = slot
	}
	return true
}

func (c *irCompiler) compileNestedLoop(loop *LoopExpr, isLast bool) bool {
	loopLet := (*LetExpr)(loop)
	baseSlot := -1

	loopFrame := -1
	for _, bodyExpr := range loopLet.body {
		if f := findBindingFrame(bodyExpr); f > c.loopFrame {
			loopFrame = f
			break
		}
	}
	if loopFrame < 0 {
		loopFrame = c.loopFrame + c.depth
	}

	// Save existing bindings for this frame to restore after scope exits.
	savedBindings := make(map[bindingKey]int)
	for key, slot := range c.bindingMap {
		if key.frame == loopFrame {
			savedBindings[key] = slot
		}
	}

	for i, bindExpr := range loopLet.values {
		if !c.compileExpr(bindExpr, false) {
			return false
		}
		// As with let, init expressions may capture outer bindings and grow
		// c.numSlots. Allocate loop slots after each init is compiled so the
		// nested loop's contiguous recur target never collides with captures.
		slot := c.numSlots
		if i == 0 {
			baseSlot = slot
		}
		c.numSlots++
		c.bindingMap[bindingKey{frame: loopFrame, index: i}] = slot
		c.emitWithOperand(irStoreSlot, slot)
	}
	if baseSlot < 0 {
		return false
	}

	loopStartPC := len(c.code)
	c.recurTargets = append(c.recurTargets, recurTarget{
		pc:       loopStartPC,
		baseSlot: baseSlot,
		nBinds:   len(loopLet.names),
	})

	for i, expr := range loopLet.body {
		if !c.compileExpr(expr, isLast && i == len(loopLet.body)-1) {
			c.recurTargets = c.recurTargets[:len(c.recurTargets)-1]
			return false
		}
	}

	c.recurTargets = c.recurTargets[:len(c.recurTargets)-1]
	// Restore outer scope bindings for this frame.
	for key := range c.bindingMap {
		if key.frame == loopFrame {
			delete(c.bindingMap, key)
		}
	}
	for key, slot := range savedBindings {
		c.bindingMap[key] = slot
	}
	return true
}

func irInlineMode() string {
	mode := os.Getenv("JOKER_IR_INLINE")
	if mode == "" {
		return "auto"
	}
	return mode
}

func irInlineForce() bool {
	mode := irInlineMode()
	return mode == "1" || mode == "force" || mode == "all"
}

func irInlineDisabled() bool {
	mode := irInlineMode()
	return mode == "0" || mode == "off" || mode == "false"
}

func exprHasTextLiteralOrStr(expr Expr) bool {
	switch e := expr.(type) {
	case *LiteralExpr:
		switch e.obj.(type) {
		case String, Char:
			return true
		}
	case *IfExpr:
		return exprHasTextLiteralOrStr(e.cond) || exprHasTextLiteralOrStr(e.positive) || exprHasTextLiteralOrStr(e.negative)
	case *LetExpr:
		for _, v := range e.values {
			if exprHasTextLiteralOrStr(v) {
				return true
			}
		}
		for _, b := range e.body {
			if exprHasTextLiteralOrStr(b) {
				return true
			}
		}
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok && coreVarToProcName(vref.vr) == "procStr" {
			return true
		}
		if exprHasTextLiteralOrStr(e.callable) {
			return true
		}
		for _, a := range e.args {
			if exprHasTextLiteralOrStr(a) {
				return true
			}
		}
	case *RecurExpr:
		for _, a := range e.args {
			if exprHasTextLiteralOrStr(a) {
				return true
			}
		}
	}
	return false
}

func exprHasCollectionOp(expr Expr) bool {
	switch e := expr.(type) {
	case *IfExpr:
		return exprHasCollectionOp(e.cond) || exprHasCollectionOp(e.positive) || exprHasCollectionOp(e.negative)
	case *LetExpr:
		for _, v := range e.values {
			if exprHasCollectionOp(v) {
				return true
			}
		}
		for _, b := range e.body {
			if exprHasCollectionOp(b) {
				return true
			}
		}
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok {
			switch coreVarToProcName(vref.vr) {
			case "procNth", "procGet", "procAssoc", "procConj", "procCount", "procFirst":
				return true
			}
		} else {
			// Calls through local helpers are not considered straight-line.
			return false
		}
		for _, a := range e.args {
			if exprHasCollectionOp(a) {
				return true
			}
		}
	case *RecurExpr:
		for _, a := range e.args {
			if exprHasCollectionOp(a) {
				return true
			}
		}
	}
	return false
}

func exprIsPureArithmetic(expr Expr) bool {
	switch e := expr.(type) {
	case *LiteralExpr:
		switch e.obj.(type) {
		case Int, Double:
			return true
		default:
			return false
		}
	case *BindingExpr:
		return true
	case *IfExpr:
		return exprIsPureArithmetic(e.cond) && exprIsPureArithmetic(e.positive) && exprIsPureArithmetic(e.negative)
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok {
			switch coreVarToProcName(vref.vr) {
			case "procAdd", "procSubtract", "procMultiply", "procDivide",
				"procInc", "procDec", "procRem", "procQuot",
				"procLt", "procGt", "procLte", "procGte", "procEq",
				"procAbs", "procMax", "procMin":
			default:
				return false
			}
		} else {
			return false
		}
		for _, a := range e.args {
			if !exprIsPureArithmetic(a) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func exprIsStraightLine(expr Expr) bool {
	switch e := expr.(type) {
	case *LoopExpr, *RecurExpr:
		return false
	case *LetExpr:
		for _, v := range e.values {
			if !exprIsStraightLine(v) {
				return false
			}
		}
		for _, b := range e.body {
			if !exprIsStraightLine(b) {
				return false
			}
		}
	case *IfExpr:
		return exprIsStraightLine(e.cond) && exprIsStraightLine(e.positive) && exprIsStraightLine(e.negative)
	case *CallExpr:
		if _, ok := e.callable.(*VarRefExpr); !ok {
			return false
		}
		for _, a := range e.args {
			if !exprIsStraightLine(a) {
				return false
			}
		}
	}
	return true
}

func exprCount(expr Expr) int {
	switch e := expr.(type) {
	case *IfExpr:
		return 1 + exprCount(e.cond) + exprCount(e.positive) + exprCount(e.negative)
	case *LetExpr:
		n := 1
		for _, v := range e.values {
			n += exprCount(v)
		}
		for _, b := range e.body {
			n += exprCount(b)
		}
		return n
	case *CallExpr:
		n := 1 + exprCount(e.callable)
		for _, a := range e.args {
			n += exprCount(a)
		}
		return n
	case *RecurExpr:
		n := 1
		for _, a := range e.args {
			n += exprCount(a)
		}
		return n
	default:
		return 1
	}
}

func (c *irCompiler) compileTryCatch(e *TryExpr, isLast bool) bool {
	// Only support single catch with no finally for now
	if len(e.catches) != 1 || len(e.finallyExpr) > 0 {
		return c.reject("IR try/catch: only single catch without finally supported")
	}
	catch := e.catches[0]

	// Emit irTryCatch with placeholder for catchPC
	catchPCIdx := len(c.code) + 1 // position where catchPC will be
	bindSlot := c.numSlots
	c.numSlots++
	c.code = append(c.code, irTryCatch, 0, 0, byte(bindSlot>>8), byte(bindSlot))

	// Compile try body
	for i, bodyExpr := range e.body {
		if !c.compileExpr(bodyExpr, isLast && i == len(e.body)-1) {
			return false
		}
	}
	if !isLast {
		// Jump over catch body
		jumpIdx := len(c.code) + 1
		c.code = append(c.code, irJump, 0, 0)
		// Patch catchPC to here
		catchPC := len(c.code)
		c.code[catchPCIdx] = byte(catchPC >> 8)
		c.code[catchPCIdx+1] = byte(catchPC)

		// Set up catch binding
		catchFrame := c.loopFrame + c.depth + 1
		c.bindingMap[bindingKey{frame: catchFrame, index: 0}] = bindSlot
		_ = catch.excSymbol

		// Compile catch body
		for i, bodyExpr := range catch.body {
			if !c.compileExpr(bodyExpr, isLast && i == len(catch.body)-1) {
				return false
			}
		}
		// Patch jump target to after catch
		afterCatch := len(c.code)
		c.code[jumpIdx] = byte(afterCatch >> 8)
		c.code[jumpIdx+1] = byte(afterCatch)
	} else {
		// isLast: try body already has irReturn
		// Patch catchPC to here for the catch handler
		catchPC := len(c.code)
		c.code[catchPCIdx] = byte(catchPC >> 8)
		c.code[catchPCIdx+1] = byte(catchPC)

		catchFrame := c.loopFrame + c.depth + 1
		c.bindingMap[bindingKey{frame: catchFrame, index: 0}] = bindSlot

		for i, bodyExpr := range catch.body {
			if !c.compileExpr(bodyExpr, i == len(catch.body)-1) {
				return false
			}
		}
	}
	return true
}
