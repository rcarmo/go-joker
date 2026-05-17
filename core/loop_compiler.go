package core

import (
	"fmt"
	coreir "github.com/rcarmo/go-joker/core/ir"
	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
	"math"
)

// ---- loop_compiler.go ----
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
	selfVar          *Var // for defn-style var-based self-calls
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
	return (&IRProgram{
		code:        c.code,
		constants:   c.constants,
		numSlots:    c.numSlots,
		captureKeys: c.captureKeys,
		fnExprs:     c.fnExprs,
	}).refreshModel(), ""
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
		case coretypes.Counted:
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
				case coretypes.Counted:
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
			res := collectionConstruction.EmptyArrayMap()
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
			if isLast {
				c.emit(irReturn)
			}
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

// ---- loop_frame_detect.go ----
// ir_frame_detect.go — precise frame detection for let/loop bindings.
//
// The IR compiler needs to know which parse-time frame each let/loop
// binding belongs to. Instead of guessing via heuristics, this scans
// the body for binding references and deduces the frame from the
// binding indices.

// findLetFrame determines the parse-time frame for a let expression's
// bindings. It scans the body for BindingExpr nodes with indices 0..nBinds-1
// that reference a frame not already known in the compiler's bindingMap.
func findLetFrame(body []Expr, nBinds int, known map[bindingKey]int) int {
	if nBinds == 0 {
		return -1
	}
	// Collect candidate frames: frames where index 0 appears and is NOT in known
	candidates := map[int]int{} // frame -> count of matching indices
	var scan func(e Expr)
	scan = func(e Expr) {
		switch x := e.(type) {
		case *BindingExpr:
			f, idx := x.binding.frame, x.binding.index
			if idx < nBinds {
				if _, alreadyKnown := known[bindingKey{frame: f, index: idx}]; !alreadyKnown {
					candidates[f]++
				}
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
		case *LetExpr:
			for _, v := range x.values {
				scan(v)
			}
			for _, b := range x.body {
				scan(b)
			}
		case *LoopExpr:
			le := (*LetExpr)(x)
			for _, v := range le.values {
				scan(v)
			}
			for _, b := range le.body {
				scan(b)
			}
		}
	}
	for _, e := range body {
		scan(e)
	}

	// Pick the candidate frame where count matches nBinds exactly
	// (the let's own frame should have exactly nBinds distinct indices)
	bestFrame := -1
	for f, count := range candidates {
		if count == nBinds {
			if bestFrame < 0 || f < bestFrame {
				bestFrame = f
			}
		}
	}
	// Fallback: pick the smallest frame with any matches
	if bestFrame < 0 {
		for f := range candidates {
			if bestFrame < 0 || f < bestFrame {
				bestFrame = f
			}
		}
	}
	return bestFrame
}

// ---- loop_native_helpers.go ----
// ir_native_helper.go — compile pure arithmetic helpers to Go closures.
//
// When a loop calls a pure arithmetic helper via irCallSlot, this path
// compiles the helper's IR to a native Go function that operates on
// float64 values directly, eliminating WASM/IR dispatch and Object boxing.

// nativeF64Fn is a compiled Go closure for a pure arithmetic helper.
type nativeF64Fn func(args []float64) float64

// nativeF64Fn2 is a 2-argument specialization that avoids slice allocation.
type nativeF64Fn2 func(a, b float64) float64

// irCompileNativeHelper attempts to compile an IR program (helper function)
// to a native Go float64 closure.
func irCompileNativeHelper(prog *IRProgram) nativeF64Fn {
	if prog == nil || prog.hasSelf {
		return nil
	}
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	// Only compile pure numeric programs (no collections, strings, calls)
	code := model.Code
	for pc := 0; pc < len(code); {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irDiv, irRem, irInc, irDec,
			irLt, irGte, irGt, irLte, irEq, irIsZero, irReturn, irSqrt:
			// ok
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				return nil
			}
		default:
			return nil
		}
	}

	// Build constants as float64
	consts := make([]float64, len(prog.constants))
	for i, c := range prog.constants {
		switch v := c.(type) {
		case Int:
			consts[i] = float64(v.I)
		case Double:
			consts[i] = v.D
		default:
			return nil
		}
	}

	numSlots := model.NumSlots
	codeSlice := model.Code

	return func(args []float64) float64 {
		var slotBuf [8]float64
		var slots []float64
		if numSlots <= len(slotBuf) {
			slots = slotBuf[:numSlots]
		} else {
			slots = make([]float64, numSlots)
		}
		copy(slots, args)

		var stack [16]float64
		sp := 0
		pc := 0

		for pc < len(codeSlice) {
			op := codeSlice[pc]
			pc++
			switch op {
			case irLiteral:
				idx := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				stack[sp] = consts[idx]
				sp++
			case irLoadSlot:
				idx := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				stack[sp] = slots[idx]
				sp++
			case irStoreSlot:
				idx := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				sp--
				slots[idx] = stack[sp]
			case irAdd:
				sp--
				stack[sp-1] += stack[sp]
			case irSub:
				sp--
				stack[sp-1] -= stack[sp]
			case irMul:
				sp--
				stack[sp-1] *= stack[sp]
			case irDiv:
				sp--
				stack[sp-1] /= stack[sp]
			case irSqrt:
				stack[sp-1] = math.Sqrt(stack[sp-1])
			case irRem:
				sp--
				b := int(stack[sp])
				if b != 0 {
					stack[sp-1] = float64(int(stack[sp-1]) % b)
				}
			case irInc:
				stack[sp-1]++
			case irDec:
				stack[sp-1]--
			case irLt:
				sp--
				if stack[sp-1] < stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irGte:
				sp--
				if stack[sp-1] >= stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irGt:
				sp--
				if stack[sp-1] > stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irLte:
				sp--
				if stack[sp-1] <= stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irEq:
				sp--
				if stack[sp-1] == stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irIsZero:
				if stack[sp-1] == 0 {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irJumpIfNot:
				target := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				sp--
				if stack[sp] == 0 {
					pc = target
				}
			case irJump:
				pc = int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
			case irRecur:
				nargs := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				target := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				for i := nargs - 1; i >= 0; i-- {
					sp--
					slots[i] = stack[sp]
				}
				pc = target
			case irReturn:
				sp--
				return stack[sp]
			default:
				return 0
			}
		}
		if sp > 0 {
			return stack[sp-1]
		}
		return 0
	}
}

// ---- loop_wasm_diagnostics.go ----
// ir_diagnostics.go — lightweight IR/WASM compilation explanations.
//
// These helpers are intentionally internal: they give benchmark and regression
// tests a stable way to answer "which execution path did this hot form take?"
// without changing Joker's public language surface. The goal is to make future
// core-runtime speed work measurable instead of guess-driven.

type IRDiagnostic struct {
	Compiled    bool
	Reason      string
	BodyIndex   int
	NumSlots    int
	NumCaptures int
	NumOps      int
	UsesWASM    bool
	WASM        corewasm.Diagnostic
	Analysis    coreir.Analysis
}

func explainIRCompile(loop *LoopExpr) IRDiagnostic {
	if loop == nil {
		return IRDiagnostic{Reason: "nil loop"}
	}
	prog, reason := irCompileExplain(loop)
	if prog == nil {
		if reason == "" {
			reason = "IR compiler rejected loop body (unsupported form or unsafe binding shape)"
		}
		return IRDiagnostic{Reason: reason}
	}
	wasm := explainWASMEligibility(prog)
	analysis := AnalyzeIRProgram(prog)
	model := prog.neutralModel()
	return IRDiagnostic{
		Compiled:    true,
		NumSlots:    model.NumSlots,
		NumCaptures: len(prog.captureKeys),
		NumOps:      coreir.OpCount(model.Code),
		UsesWASM:    wasm.Eligible && !wasm.HasImports,
		WASM:        wasm,
		Analysis:    analysis,
	}
}

func explainWASMEligibility(prog *IRProgram) corewasm.Diagnostic {
	if prog == nil {
		return corewasm.Diagnostic{Reason: "nil IR program"}
	}
	model := prog.neutralModel()
	if model == nil {
		return corewasm.Diagnostic{Reason: "nil IR program model"}
	}
	return corewasm.ExplainEligibility(model.Code, len(model.FloatConsts) > 0)
}

func findFirstLoopExpr(expr Expr) *LoopExpr {
	switch e := expr.(type) {
	case *LoopExpr:
		return e
	case *LetExpr:
		for _, v := range e.values {
			if loop := findFirstLoopExpr(v); loop != nil {
				return loop
			}
		}
		for _, b := range e.body {
			if loop := findFirstLoopExpr(b); loop != nil {
				return loop
			}
		}
	case *IfExpr:
		if loop := findFirstLoopExpr(e.cond); loop != nil {
			return loop
		}
		if loop := findFirstLoopExpr(e.positive); loop != nil {
			return loop
		}
		return findFirstLoopExpr(e.negative)
	case *CallExpr:
		if loop := findFirstLoopExpr(e.callable); loop != nil {
			return loop
		}
		for _, a := range e.args {
			if loop := findFirstLoopExpr(a); loop != nil {
				return loop
			}
		}
	case *RecurExpr:
		for _, a := range e.args {
			if loop := findFirstLoopExpr(a); loop != nil {
				return loop
			}
		}
	}
	return nil
}

func explainFirstLoop(expr Expr) IRDiagnostic {
	loop := findFirstLoopExpr(expr)
	if loop == nil {
		return IRDiagnostic{Reason: "no loop expression found"}
	}
	return explainIRCompile(loop)
}

// ---- inline_rewrites.go ----
func (c *irCompiler) tryInlineCall(fnSlot int, expr *CallExpr, isLast bool) bool {
	_ = fnSlot
	if corert.IRInlineDisabled() {
		return false
	}
	fnExpr := findFnExprForBinding(expr.callable)
	if fnExpr == nil || len(fnExpr.arities) != 1 || fnExpr.variadic != nil {
		return false
	}
	arity := fnExpr.arities[0]
	if !corert.IRInlineForce() {
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

	// Check for var-based self-recursive call (defn fib [...] (fib ...))
	if c.hasSelf && c.selfVar != nil && vref.vr == c.selfVar && len(expr.args) == c.selfNArgs {
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
	case "procBitAnd":
		if len(expr.args) != 2 {
			return c.reject("bit-and expects 2 args")
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		if !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irBitAnd)
	case "procBitOr":
		if len(expr.args) != 2 {
			return c.reject("bit-or expects 2 args")
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		if !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irBitOr)
	case "procBitNot":
		if len(expr.args) != 1 {
			return c.reject("bit-not expects 1 arg")
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irBitNot)
	case "procBitShiftLeft":
		if len(expr.args) != 2 {
			return c.reject("bit-shift-left expects 2 args")
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		if !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irBitShiftLeft)
	case "procBitShiftRight":
		if len(expr.args) != 2 {
			return c.reject("bit-shift-right expects 2 args")
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		if !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irBitShiftRight)
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
	case "<=":
		return "procLte"
	case ">":
		return "procGt"
	case ">=":
		return "procGte"
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
