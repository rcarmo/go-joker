package core

import (
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
	irIntCast                    // pop 1 (Char or Number), push Int
	irSubs                       // pop 2 or 3 (string, start [, end]), push substring
	irGte                        // pop 2, push a >= b
	irGt                         // pop 2, push a > b
	irLte                        // pop 2, push a <= b
	irCursorChar                 // pop cursor, push char (rune as Char)
	irCursorNext                 // pop cursor, push new cursor (advanced by 1)
	irCursorDone                 // pop cursor, push boolean (done?)
	irPackRest                   // operand: startIdx — pack slots[startIdx:nargs] into vector, store to slot
	irApply                      // pop fn + args-seq, call fn with unpacked args, push result
	irThrow                      // pop value, panic with it
	irTryCatch                   // operands: catchPC(2) + bindSlot(2) — set up catch handler
	irPop                        // pop and discard top of stack
	irMakeFn                     // operand: constant index (FnExpr) — creates *Fn with current env
	irCase                       // operands: slot(2) + nCases(2) + [value(2)+targetPC(2)]*n + defaultPC(2)
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
	code            []byte
	constants       []Object
	numSlots        int
	captureKeys     []bindingKey
	captureSlots    []Object // resolved capture values from fn.env
	captureSlotIdxs []int    // slot indices for each capture
	hasSelf         bool
	escapeInfo      *EscapeInfo
	analysis        *IRAnalysis
	typedFailed     bool
	memNthFailed    bool
	nativeHelper    nativeF64Fn
	nativeHelper2   nativeF64Fn2
	nativeChecked   bool
	floatConsts     []float64
	// Multi-arity support: map from arg count to sub-program
	arityPrograms   map[int]*IRProgram
	variadicProg    *IRProgram // for variadic arity (min args)
	variadicMinArgs int
	fnExprs         []*FnExpr // for irMakeFn opcode
}
