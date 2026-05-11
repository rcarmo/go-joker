package core

import (
	"sync"

	coreir "github.com/rcarmo/go-joker/core/internal/ir"
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
	irLiteral        = coreir.Literal        // operand: index into constants pool
	irLoadSlot       = coreir.LoadSlot       // operand: slot index in locals
	irStoreSlot      = coreir.StoreSlot      // operand: slot index in locals
	irAdd            = coreir.Add            // pop 2, push sum (Int fast path)
	irSub            = coreir.Sub            // pop 2, push difference
	irMul            = coreir.Mul            // pop 2, push product
	irRem            = coreir.Rem            // pop 2, push remainder
	irDiv            = coreir.Div            // pop 2, push quotient (Double)
	irInc            = coreir.Inc            // pop 1, push +1
	irDec            = coreir.Dec            // pop 1, push -1
	irLt             = coreir.Lt             // pop 2, push Boolean
	irEq             = coreir.Eq             // pop 2, push Boolean
	irIsZero         = coreir.IsZero         // pop 1, push Boolean
	irJumpIfNot      = coreir.JumpIfNot      // operand: target PC (uint16 big-endian in next 2 bytes)
	irJump           = coreir.Jump           // operand: target PC
	irRecur          = coreir.Recur          // operand: nargs (2 bytes) + target PC (2 bytes)
	irReturn         = coreir.Return         // pop 1, return it
	irGet            = coreir.Get            // pop 2 (coll, key), push result or NIL
	irGet3           = coreir.Get3           // pop 3 (coll, key, default), push result
	irAssoc          = coreir.Assoc          // pop 3 (coll, key, val), push new map
	irNth            = coreir.Nth            // pop 2 (coll, index), push element
	irConj           = coreir.Conj           // pop 2 (coll, val), push conj'd
	irSqrt           = coreir.Sqrt           // pop 1, push sqrt
	irCallSlot       = coreir.CallSlot       // operand1: slot (2 bytes), operand2: nargs (2 bytes)
	irCallSelf       = coreir.CallSelf       // operand: nargs (2 bytes)
	irFirst          = coreir.First          // pop 1, push first element
	irBuildVec       = coreir.BuildVec       // operand: n elements; pop n, push new vector
	irStr2           = coreir.Str2           // pop 2, push string concatenation
	irStr1           = coreir.Str1           // pop 1, push string conversion
	irNthStringASCII = coreir.NthStringASCII // operand: constant string index; pop idx, push char
	irCount          = coreir.Count          // pop 1, push count
	irToTransient    = coreir.ToTransient    // pop 1 (ArrayVector), push TransientVector
	irAssocBang      = coreir.AssocBang      // pop 3 (tv, key, val), mutate in place, push tv
	irToPersistent   = coreir.ToPersistent   // pop 1 (TransientVector), push ArrayVector
	irFallback       = coreir.Fallback       // cannot execute in IR; fall back to tree Eval
	irIntCast        = coreir.IntCast        // pop 1 (Char or Number), push Int
	irSubs           = coreir.Subs           // pop 2 or 3 (string, start [, end]), push substring
	irGte            = coreir.Gte            // pop 2, push a >= b
	irGt             = coreir.Gt             // pop 2, push a > b
	irLte            = coreir.Lte            // pop 2, push a <= b
	irCursorChar     = coreir.CursorChar     // pop cursor, push char (rune as Char)
	irCursorNext     = coreir.CursorNext     // pop cursor, push new cursor (advanced by 1)
	irCursorDone     = coreir.CursorDone     // pop cursor, push boolean (done?)
	irPackRest       = coreir.PackRest       // operand: startIdx — pack slots[startIdx:nargs] into vector, store to slot
	irApply          = coreir.Apply          // pop fn + args-seq, call fn with unpacked args, push result
	irThrow          = coreir.Throw          // pop value, panic with it
	irTryCatch       = coreir.TryCatch       // operands: catchPC(2) + bindSlot(2) — set up catch handler
	irPop            = coreir.Pop            // pop and discard top of stack
	irMakeFn         = coreir.MakeFn         // operand: constant index (FnExpr) — creates *Fn with current env
	irCase           = coreir.Case           // operands: slot(2) + nCases(2) + [value(2)+targetPC(2)]*n + defaultPC(2)
	irBitAnd         = coreir.BitAnd         // pop 2, push a & b
	irBitOr          = coreir.BitOr          // pop 2, push a | b
	irBitNot         = coreir.BitNot         // pop 1, push ^a
	irBitShiftLeft   = coreir.BitShiftLeft   // pop 2, push a << b
	irBitShiftRight  = coreir.BitShiftRight  // pop 2, push a >> b (arithmetic)
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
	model           *coreir.Program
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
	execFailed      bool // both typed AND boxed failed — skip IR entirely
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
	traceName       string
	captureSlotSet  []bool // captureSlotSet[i] = true if slot i holds a capture (skip clearing)
}

func (p *IRProgram) neutralModel() *coreir.Program {
	if p == nil {
		return nil
	}
	if p.model == nil {
		p.refreshModel()
	}
	return p.model
}

func (p *IRProgram) refreshModel() *IRProgram {
	if p == nil {
		return nil
	}
	model := coreir.NewProgram(p.code, p.numSlots, len(p.constants))
	model.HasSelf = p.hasSelf
	model.FloatConsts = append([]float64(nil), p.floatConsts...)
	model.WithCaptures(p.captureSlotIdxs, p.captureSlotSet)
	if p.analysis != nil {
		analysis := coreir.Analyze(p.code, p.numSlots, len(p.captureKeys), irProgramUsesFloat(p), p.analysis.StringAppendSlots, p.analysis.StringPrependSlots)
		model.Analysis = &analysis
	}
	if len(p.arityPrograms) > 0 || p.variadicProg != nil || p.variadicMinArgs != 0 {
		arityPrograms := make(map[int]*coreir.Program, len(p.arityPrograms))
		for arity, prog := range p.arityPrograms {
			if prog != nil {
				arityPrograms[arity] = prog.refreshModel().model
			}
		}
		var variadic *coreir.Program
		if p.variadicProg != nil {
			variadic = p.variadicProg.refreshModel().model
		}
		model.WithArityPrograms(arityPrograms, variadic, p.variadicMinArgs)
	}
	p.model = model
	return p
}
