package core

import (
	"fmt"
	"math"
	"os"
	"sync"
)

// ir.go — tiny lowered IR for hot loop/arithmetic subsets.
//
// The IR represents a small subset of Joker expressions as a flat
// instruction sequence with slot-resolved locals. It is interpreted
// by a simple switch loop that avoids the overhead of tree-walking
// Eval, interface dispatch, defer, and frame allocation.
//
// The IR is lowered lazily from LoopExpr bodies when all contained
// expressions fall within the supported subset. Compiled programs
// are cached so the compile cost is only paid once per loop site.

// Opcodes
const (
	irLiteral        byte = iota // operand: index into constants pool
	irLoadSlot                   // operand: slot index in locals
	irStoreSlot                  // operand: slot index in locals
	irAdd                        // pop 2, push sum (Int fast path)
	irSub                        // pop 2, push difference
	irMul                        // pop 2, push product
	irRem                        // pop 2, push remainder
	irDiv                        // pop 2, push quotient (Double)
	irInc                        // pop 1, push +1
	irDec                        // pop 1, push -1
	irLt                         // pop 2, push Boolean
	irEq                         // pop 2, push Boolean
	irIsZero                     // pop 1, push Boolean
	irJumpIfNot                  // operand: target PC (uint16 big-endian in next 2 bytes)
	irJump                       // operand: target PC
	irRecur                      // operand: nargs (2 bytes) + target PC (2 bytes)
	irReturn                     // pop 1, return it
	irGet                        // pop 2 (coll, key), push result or NIL
	irGet3                       // pop 3 (coll, key, default), push result
	irAssoc                      // pop 3 (coll, key, val), push new map
	irNth                        // pop 2 (coll, index), push element
	irConj                       // pop 2 (coll, val), push conj'd
	irSqrt                       // pop 1, push sqrt
	irCallSlot                   // operand1: slot (2 bytes), operand2: nargs (2 bytes)
	irCallSelf                   // operand: nargs (2 bytes)
	irFirst                      // pop 1, push first element
	irBuildVec                   // operand: n elements; pop n, push new vector
	irStr2                       // pop 2, push string concatenation
	irStr1                       // pop 1, push string conversion
	irNthStringASCII             // operand: constant string index; pop idx, push char
	irCount                      // pop 1, push count
	irToTransient                // pop 1 (ArrayVector), push TransientVector
	irAssocBang                  // pop 3 (tv, key, val), mutate in place, push tv
	irToPersistent               // pop 1 (TransientVector), push ArrayVector
	irFallback                   // cannot execute in IR; fall back to tree Eval
)

// ---------- Cache ----------

var irCache sync.Map   // map[*LoopExpr]*IRProgram
var irFnCache sync.Map // map[*FnArityExpr]*IRProgram

var irCompileFailed = &IRProgram{} // sentinel

func irGetCached(loop *LoopExpr) *IRProgram {
	if cached, ok := irCache.Load(loop); ok {
		prog := cached.(*IRProgram)
		if prog == irCompileFailed {
			return nil
		}
		return prog
	}
	prog := irCompile(loop)
	if prog == nil {
		irCache.Store(loop, irCompileFailed)
		return nil
	}
	irCache.Store(loop, prog)
	return prog
}

// ---------- Program ----------

type IRProgram struct {
	code        []byte
	constants   []Object
	numSlots    int
	captureKeys []bindingKey
	hasSelf     bool
	escapeInfo  *EscapeInfo
	analysis    *IRAnalysis
	typedFailed bool
}

// ---------- Fn compilation ----------

// irCompileFn attempts to compile a single-arity Fn body into an IRProgram.
// The args become slots 0..n-1. Returns nil if the body can't be compiled.
// selfBinding optionally identifies the binding key for self-recursive calls.
func irCompileFn(fn *Fn) *IRProgram {
	if len(fn.fnExpr.arities) != 1 {
		return nil
	}
	arity := fn.fnExpr.arities[0]

	if cached, ok := irFnCache.Load(&arity); ok {
		prog := cached.(*IRProgram)
		if prog == irCompileFailed {
			return nil
		}
		return prog
	}

	c := &irCompiler{
		bindingMap: make(map[bindingKey]int),
		loopFrame:  -1,
	}
	c.numSlots = len(arity.args)

	// Determine the frame from the body
	fnFrame := guessLoopFrame(arity.body)
	if fnFrame < 0 {
		fnFrame = 1
	}
	c.loopFrame = fnFrame
	for i := range arity.args {
		c.bindingMap[bindingKey{frame: fnFrame, index: i}] = i
	}

	// If the fn is tail-rewritten, its body has RecurExpr nodes
	// that need a recur target (like a loop body)
	if fn.fnExpr.tailRewritten {
		c.recurTargets = []recurTarget{{pc: 0, baseSlot: 0, nBinds: len(arity.args)}}
	}

	// If the fn has a self-binding, mark it for self-recursive IR dispatch
	if fn.fnExpr.self.name != nil {
		// The self-binding is typically at frame fnFrame-1, index 0
		// (the letfn/fn frame that holds the fn itself)
		c.selfSlot = 0 // will use special irCallSelf opcode
		c.hasSelf = true
		c.selfNArgs = len(arity.args)
	}

	// Compile body
	for i, expr := range arity.body {
		if !c.compileExpr(expr, i == len(arity.body)-1) {
			irFnCache.Store(&arity, irCompileFailed)
			return nil
		}
	}
	if len(c.code) == 0 {
		irFnCache.Store(&arity, irCompileFailed)
		return nil
	}
	if c.code[len(c.code)-1] != irReturn {
		c.emit(irReturn)
	}
	// Allow self-captures but no other captures
	if len(c.captureKeys) > 0 {
		irFnCache.Store(&arity, irCompileFailed)
		return nil
	}
	prog := &IRProgram{
		code:      c.code,
		constants: c.constants,
		numSlots:  c.numSlots,
		hasSelf:   c.hasSelf,
	}
	irFnCache.Store(&arity, prog)
	return prog
}

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
	numSlots         int
	loopFrame        int
	depth            int
	hasSelf          bool
	selfSlot         int
	selfNArgs        int
	recurTargets     []recurTarget
	rejectReason     string
	hasCollectionOps bool
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
		if c.depth > 4 {
			return c.reject("IR nesting depth exceeded for let: %d > 4", c.depth)
		}
		c.depth++
		return c.compileLetBody(e, isLast)

	case *LoopExpr:
		if c.depth > 4 {
			return c.reject("IR nesting depth exceeded for nested loop: %d > 4", c.depth)
		}
		c.depth++
		return c.compileNestedLoop(e, isLast)

	default:
		return c.reject("unsupported IR expression type %T", expr)
	}
}

func (c *irCompiler) compileLetBody(e *LetExpr, isLast bool) bool {
	letFrame := -1
	for _, bodyExpr := range e.body {
		if f := findBindingFrame(bodyExpr); f > c.loopFrame {
			letFrame = f
			break
		}
	}
	if letFrame < 0 {
		letFrame = c.loopFrame + c.depth
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
				// caller loop has no collection ops. If the caller uses
				// nth/get/etc, the helper likely runs faster as standalone
				// WASM via irCallSlot.
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
	baseSlot := c.numSlots
	for _, arg := range expr.args {
		if !c.compileExpr(arg, false) {
			return false
		}
	}
	oldBindings := make(map[bindingKey]int, len(arity.args))
	oldPresent := make(map[bindingKey]bool, len(arity.args))
	for i := len(arity.args) - 1; i >= 0; i-- {
		slot := baseSlot + i
		key := bindingKey{frame: fnFrame, index: i}
		if old, ok := c.bindingMap[key]; ok {
			oldBindings[key] = old
			oldPresent[key] = true
		}
		c.bindingMap[key] = slot
		c.emitWithOperand(irStoreSlot, slot)
	}
	c.numSlots = baseSlot + len(arity.args)
	ok := c.compileExpr(arity.body[0], isLast)
	for i := range arity.args {
		key := bindingKey{frame: fnFrame, index: i}
		if oldPresent[key] {
			c.bindingMap[key] = oldBindings[key]
		} else {
			delete(c.bindingMap, key)
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
	case "procEq":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irEq)
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
	default:
		return ""
	}
}

// ---------- Interpreter ----------

func irExec(prog *IRProgram, initSlots []Object) Object {
	var slots []Object
	if prog.numSlots <= 16 {
		var buf [16]Object
		slots = buf[:prog.numSlots]
	} else {
		slots = make([]Object, prog.numSlots)
	}
	copy(slots, initSlots)

	// Escape analysis: convert safe local values to transient builders.
	// Only run if there are actually mutable candidate slots.
	hasMutableCandidate := false
	for _, s := range slots {
		switch s.(type) {
		case *ArrayVector, *ArrayMap, *HashMap, String:
			hasMutableCandidate = true
		}
		if hasMutableCandidate {
			break
		}
	}
	if hasMutableCandidate {
		if prog.escapeInfo == nil {
			prog.escapeInfo = analyzeEscapes(prog)
		}
		for i, s := range slots {
			if !prog.escapeInfo.SafeMutableSlots[i] {
				continue
			}
			switch v := s.(type) {
			case *ArrayVector:
				slots[i] = ToTransient(v)
			case *ArrayMap:
				slots[i] = MapToTransient(v)
			case *HashMap:
				slots[i] = MapToTransient(v)
			case String:
				if !irStringBuilderDisabled() {
					if irStringBuilderForce() && (prog.escapeInfo.StringBuilderSlots[i] || prog.escapeInfo.StringPrependSlots[i]) {
						slots[i] = ToTransientString(v)
					} else if !irStringBuilderForce() && prog.escapeInfo.StringPrependSlots[i] {
						slots[i] = ToTransientString(v)
					}
				}
			}
		}
	}

	var stack []Object
	var stackBuf [16]Object
	stack = stackBuf[:0]
	code := prog.code
	pc := 0

loop:
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stack = append(stack, prog.constants[idx])

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stack = append(stack, slots[idx])

		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			slots[idx] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]

		case irAdd:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Int{I: av.I + bv.I})
					continue
				case Double:
					stack = append(stack, Double{D: float64(av.I) + bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Double{D: av.D + float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Double{D: av.D + bv.D})
					continue
				}
			}
			return nil

		case irSub:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Int{I: av.I - bv.I})
					continue
				case Double:
					stack = append(stack, Double{D: float64(av.I) - bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Double{D: av.D - float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Double{D: av.D - bv.D})
					continue
				}
			}
			return nil

		case irMul:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Int{I: av.I * bv.I})
					continue
				case Double:
					stack = append(stack, Double{D: float64(av.I) * bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Double{D: av.D * float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Double{D: av.D * bv.D})
					continue
				}
			}
			return nil

		case irRem:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if ai, aok := a.(Int); aok {
				if bi, bok := b.(Int); bok {
					if bi.I == 0 {
						return nil
					}
					stack = append(stack, Int{I: ai.I % bi.I})
					continue
				}
			}
			return nil

		case irDiv:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			var av, bv float64
			switch x := a.(type) {
			case Int:
				av = float64(x.I)
			case Double:
				av = x.D
			default:
				return nil
			}
			switch x := b.(type) {
			case Int:
				bv = float64(x.I)
			case Double:
				bv = x.D
			default:
				return nil
			}
			if bv == 0 {
				return nil
			}
			stack = append(stack, Double{D: av / bv})

		case irInc:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case Int:
				stack = append(stack, Int{I: av.I + 1})
			case Double:
				stack = append(stack, Double{D: av.D + 1})
			default:
				return nil
			}

		case irDec:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case Int:
				stack = append(stack, Int{I: av.I - 1})
			case Double:
				stack = append(stack, Double{D: av.D - 1})
			default:
				return nil
			}

		case irLt:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.I < bv.I})
					continue
				case Double:
					stack = append(stack, Boolean{B: float64(av.I) < bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.D < float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Boolean{B: av.D < bv.D})
					continue
				}
			}
			return nil

		case irEq:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.I == bv.I})
					continue
				case Double:
					stack = append(stack, Boolean{B: float64(av.I) == bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.D == float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Boolean{B: av.D == bv.D})
					continue
				}
			case Char:
				if bv, ok := b.(Char); ok {
					stack = append(stack, Boolean{B: av.Ch == bv.Ch})
					continue
				}
			case String:
				if bv, ok := b.(String); ok {
					stack = append(stack, Boolean{B: av.S == bv.S})
					continue
				}
			}
			stack = append(stack, Boolean{B: a.Equals(b)})

		case irIsZero:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case Int:
				stack = append(stack, Boolean{B: av.I == 0})
			case Double:
				stack = append(stack, Boolean{B: av.D == 0})
			default:
				return nil
			}

		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			val := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch v := val.(type) {
			case Nil:
				pc = target
			case Boolean:
				if !v.B {
					pc = target
				}
			}

		case irJump:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc = target

		case irRecur:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			targetPC := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// For nested loops, the recur target baseSlot might not be 0
			// We need to figure out which slots to write to.
			// Convention: recur writes to slots starting from the baseSlot
			// of the loop that recur targets. For the top-level loop, baseSlot=0.
			// For nested loops, we determine baseSlot from the target PC.
			// Simple approach: if targetPC==0, write to slots 0..n-1 (backward compat).
			// Otherwise, we need the baseSlot encoded somewhere.
			// For now, recur always writes to the slots at the end of stack in order.
			if targetPC == 0 {
				// Top-level loop recur
				for i := n - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			} else {
				// Nested loop recur — find the base slot from the compile info
				// We store base slot in the bytecode too: nargs(2) + targetPC(2) + baseSlot(2)
				// ... but we didn't emit baseSlot yet. Let's add it.
				// For now, infer: the slots for this loop start at (numSlots - n) or
				// we need to extend the encoding.
				// Quick fix: also encode baseSlot
				baseSlot := int(code[pc])<<8 | int(code[pc+1])
				pc += 2
				for i := n - 1; i >= 0; i-- {
					slots[baseSlot+i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			}
			pc = targetPC
			stack = stack[:0]
			goto loop

		case irReturn:
			if len(stack) == 0 {
				return NIL
			}
			result := stack[len(stack)-1]
			// Freeze any transients before returning
			switch v := result.(type) {
			case *TransientVector:
				return v.ToPersistent()
			case *TransientMap:
				return v.ToPersistent()
			case *TransientString:
				return v.ToPersistent()
			}
			return result

		case irGet:
			key := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if g, ok := coll.(Gettable); ok {
				ok, v := g.Get(key)
				if ok {
					stack = append(stack, v)
				} else {
					stack = append(stack, NIL)
				}
			} else {
				return nil
			}

		case irGet3:
			def := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if g, ok := coll.(Gettable); ok {
				ok, v := g.Get(key)
				if ok {
					stack = append(stack, v)
				} else {
					stack = append(stack, def)
				}
			} else {
				stack = append(stack, def)
			}

		case irAssoc:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			// Fast path: transient in-place mutation
			if tv, ok := coll.(*TransientVector); ok {
				stack = append(stack, tv.AssocInPlace(key, val))
			} else if tm, ok := coll.(*TransientMap); ok {
				stack = append(stack, tm.AssocInPlace(key, val))
			} else if a, ok := coll.(Associative); ok {
				stack = append(stack, a.Assoc(key, val))
			} else {
				return nil
			}

		case irNth:
			idxObj := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			idx, iok := idxObj.(Int)
			if !iok {
				return nil
			}
			switch c := coll.(type) {
			case *ArrayVector:
				if idx.I >= 0 && idx.I < len(c.arr) {
					stack = append(stack, c.arr[idx.I])
				} else {
					return nil
				}
			case *TransientVector:
				stack = append(stack, c.Nth(idx.I))
			case String:
				stack = append(stack, stringNthFast(c.S, idx.I))
			case Indexed:
				stack = append(stack, c.Nth(idx.I))
			default:
				return nil
			}

		case irConj:
			val := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch c := coll.(type) {
			case *TransientVector:
				stack = append(stack, c.ConjInPlace(val))
			case Conjable:
				stack = append(stack, c.Conj(val))
			default:
				return nil
			}

		case irSqrt:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case Double:
				stack = append(stack, Double{D: math.Sqrt(av.D)})
			case Int:
				stack = append(stack, Double{D: math.Sqrt(float64(av.I))})
			default:
				return nil
			}

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnObj := slots[slotIdx]
			var args []Object
			var argsBuf [4]Object
			if nargs > 0 {
				if nargs <= len(argsBuf) {
					args = argsBuf[:nargs]
				} else {
					args = make([]Object, nargs)
				}
				for i := nargs - 1; i >= 0; i-- {
					args[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			}
			// Try WASM fn dispatch first, then IR, then tree-walker
			if fn, ok := fnObj.(*Fn); ok {
				// Try WASM native
				if wp := wasmGetFn(fn); wp != nil {
					if result := wasmExec(wp, args); result != nil {
						stack = append(stack, result)
						continue
					}
				}
				// Try IR
				if fnProg := irCompileFn(fn); fnProg != nil {
					if result := irExec(fnProg, args); result != nil {
						stack = append(stack, result)
						continue
					}
				}
			}
			// Fallback to normal Fn.Call
			prevExpr := RT.currentExpr
			RT.currentExpr = &CallExpr{}
			switch fn := fnObj.(type) {
			case Callable:
				stack = append(stack, fn.Call(args))
			default:
				RT.currentExpr = prevExpr
				return nil
			}
			RT.currentExpr = prevExpr

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			var args []Object
			var argsBuf [4]Object
			if nargs <= len(argsBuf) {
				args = argsBuf[:nargs]
			} else {
				args = make([]Object, nargs)
			}
			for i := nargs - 1; i >= 0; i-- {
				args[i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			result := irExec(prog, args)
			if result == nil {
				return nil
			}
			stack = append(stack, result)

		case irFirst:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch v := a.(type) {
			case *ArrayVector:
				if len(v.arr) > 0 {
					stack = append(stack, v.arr[0])
				} else {
					stack = append(stack, NIL)
				}
			case Seqable:
				s := v.Seq()
				if s.IsEmpty() {
					stack = append(stack, NIL)
				} else {
					stack = append(stack, s.First())
				}
			default:
				return nil
			}

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			arr := make([]Object, n)
			for i := n - 1; i >= 0; i-- {
				arr[i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, &ArrayVector{arr: arr})

		case irStr1:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case Nil:
				stack = append(stack, String{S: ""})
			case String:
				stack = append(stack, av)
			case Char:
				stack = append(stack, charToStringObjectFast(av.Ch))
			default:
				stack = append(stack, String{S: a.ToString(false)})
			}

		case irNthStringASCII:
			idxConst := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			idxObj := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			idx, ok := idxObj.(Int)
			if !ok {
				return nil
			}
			s := prog.constants[idxConst].(String).S
			if idx.I < 0 || idx.I >= len(s) {
				return nil
			}
			stack = append(stack, Char{Ch: rune(s[idx.I])})

		case irStr2:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case *TransientString:
				switch bv := b.(type) {
				case Char:
					stack = append(stack, av.AppendChar(bv.Ch))
				case String:
					stack = append(stack, av.AppendString(bv.S))
				default:
					stack = append(stack, av.AppendString(b.ToString(false)))
				}
			case String:
				switch bv := b.(type) {
				case Char:
					stack = append(stack, String{S: av.S + charToStringFast(bv.Ch)})
				case String:
					stack = append(stack, String{S: av.S + bv.S})
				case *TransientString:
					stack = append(stack, bv.PrependString(av.S))
				default:
					stack = append(stack, String{S: av.S + b.ToString(false)})
				}
			case Char:
				if bv, ok := b.(*TransientString); ok {
					stack = append(stack, bv.PrependChar(av.Ch))
				} else {
					stack = append(stack, String{S: charToStringFast(av.Ch) + b.ToString(false)})
				}
			default:
				stack = append(stack, String{S: a.ToString(false) + b.ToString(false)})
			}

		case irCount:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch v := a.(type) {
			case *TransientString:
				stack = append(stack, Int{I: v.Count()})
			case Counted:
				stack = append(stack, Int{I: v.Count()})
			case String:
				stack = append(stack, Int{I: len(v.S)})
			default:
				return nil
			}

		case irToTransient:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if av, ok := a.(*ArrayVector); ok {
				stack = append(stack, ToTransient(av))
			} else {
				return nil
			}

		case irAssocBang:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if tv, ok := coll.(*TransientVector); ok {
				stack = append(stack, tv.AssocInPlace(key, val))
			} else {
				return nil
			}

		case irToPersistent:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if tv, ok := a.(*TransientVector); ok {
				stack = append(stack, tv.ToPersistent())
			} else {
				return nil
			}
		case irFallback:
			return nil

		default:
			return nil
		}
	}
	if len(stack) > 0 {
		return stack[len(stack)-1]
	}
	return NIL
}
