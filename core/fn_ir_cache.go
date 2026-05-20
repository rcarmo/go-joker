package core

import (
	"context"
	"encoding/binary"
	"fmt"
	corert "github.com/rcarmo/go-joker/core/runtime"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	"math"
	"reflect"
	"sync"
	"sync/atomic"

	coreir "github.com/rcarmo/go-joker/core/ir"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// ---- fn_ir_cache.go ----
// irGetCachedFnProg returns the IR program for fn only if already compiled
// and cached. Never triggers compilation. Safe to call during parse time.
func irGetCachedFnProg(fn *Fn) *IRProgram {
	if atomic.LoadUint32(&fn.irProgOnce) != 1 {
		return nil
	}
	if fn.irProg == irCompileFailed {
		return nil
	}
	return fn.irProg
}

// irGetFnProg returns the cached IR program for a Fn, compiling on first access.
// Uses atomic flag for lock-free single-check.
func irGetFnProg(fn *Fn) *IRProgram {
	if atomic.LoadUint32(&fn.irProgOnce) == 1 {
		if fn.irProg == irCompileFailed {
			return nil
		}
		return fn.irProg
	}
	// Check arity-level cache first to avoid recompiling per-instance
	if len(fn.fnExpr.arities) == 1 {
		if cached, ok := irFnCache.Load(&fn.fnExpr.arities[0]); ok {
			prog := cached.(*IRProgram)
			if prog == irCompileFailed {
				fn.irProg = irCompileFailed
				atomic.StoreUint32(&fn.irProgOnce, 1)
				return nil
			}
			// Arity cache has a prog, but it might have wrong captures
			// for this instance. Only reuse if no captures.
			if len(prog.captureSlots) == 0 {
				fn.irProg = prog
				atomic.StoreUint32(&fn.irProgOnce, 1)
				return prog
			}
		}
	}
	prog := irCompileFn(fn)
	if prog == nil {
		// irCompileFn fails for fns with captures. But if the fn body is
		// a single LoopExpr, the loop's IR program may have a native helper.
		// Compile the loop separately and use its native helper on a wrapper.
		if len(fn.fnExpr.arities) == 1 {
			arity := fn.fnExpr.arities[0]
			if len(arity.body) == 1 {
				if loop, ok := arity.body[0].(*LoopExpr); ok {
					loopProg := irCompile(loop)
					if runtimeExec.HasNativeHelper(loopProg) {
						wrapper := buildNativeLoopWrapper(fn, arity, loop, loopProg)
						if wrapper != nil {
							prog = (&IRProgram{
								numSlots:  len(arity.args),
								traceName: fn.fnExpr.traceName,
							}).refreshModel()
							runtimeExec.InstallNativeHelper(prog, wrapper)
						}
					}
				}
			}
		}
	}
	if prog == nil {
		fn.irProg = irCompileFailed
	} else {
		fn.irProg = prog
	}
	atomic.StoreUint32(&fn.irProgOnce, 1)
	return prog
}

// buildNativeLoopWrapper builds a native f64 wrapper for a fn whose body
// is a single loop. Resolves captures from both fn params (dynamic) and
// outer scope (constant, resolved from fn.env at wrapper creation time).
func buildNativeLoopWrapper(fn *Fn, arity FnArityExpr, loop *LoopExpr, loopProg *IRProgram) nativeF64Fn {
	le := (*LetExpr)(loop)
	nLoopBinds := len(le.names)
	nSlots := loopProg.numSlots
	capKeys := loopProg.captureKeys

	// Pre-compute loop init values (must be numeric literals)
	initVals := make([]float64, nLoopBinds)
	for i, v := range le.values {
		lit, ok := v.(*LiteralExpr)
		if !ok {
			return nil
		}
		switch lv := lit.obj.(type) {
		case coretypes.Int:
			initVals[i] = float64(lv.I)
		case coretypes.Double:
			initVals[i] = lv.D
		default:
			return nil
		}
	}

	// Identify fn param frame: the frame that has indices 0..len(args)-1
	// used as captures. Multiple captures from same frame with valid param indices.
	fnParamFrame := -1
	for _, ck := range capKeys {
		if ck.index < len(arity.args) {
			if fnParamFrame < 0 {
				fnParamFrame = ck.frame
			} else if fnParamFrame != ck.frame {
				// Conflicting frames — can't determine param frame
				break
			}
		}
	}
	if fnParamFrame < 0 {
		return nil
	}

	// Classify captures
	type capInfo struct {
		isDynamic bool
		argIdx    int
		constVal  float64
	}
	caps := make([]capInfo, len(capKeys))
	for ci, ck := range capKeys {
		if ck.frame == fnParamFrame && ck.index < len(arity.args) {
			caps[ci] = capInfo{isDynamic: true, argIdx: ck.index}
		} else if ck.frame == fnParamFrame {
			// Same frame as params but index >= nparams: let binding inside fn body.
			// The loop native helper will compute it; use 0 as placeholder.
			caps[ci] = capInfo{constVal: 0}
		} else {
			// Try to resolve from fn's env chain by walking parents
			resolved := false
			e := fn.env
			for e != nil {
				if ck.index < len(e.bindings) {
					obj := e.bindings[ck.index]
					switch v := obj.(type) {
					case coretypes.Int:
						caps[ci] = capInfo{constVal: float64(v.I)}
						resolved = true
					case coretypes.Double:
						caps[ci] = capInfo{constVal: v.D}
						resolved = true
					case *Fn:
						caps[ci] = capInfo{constVal: 0}
						resolved = true
					}
					if resolved {
						break
					}
				}
				e = e.parent
			}
			if !resolved {
				return nil
			}
		}
	}

	loopNative, ok := runtimeExec.NativeHelper(loopProg)
	if !ok {
		return nil
	}
	return func(fnArgs []float64) float64 {
		var buf [16]float64
		var loopArgs []float64
		if nSlots <= len(buf) {
			loopArgs = buf[:nSlots]
		} else {
			loopArgs = make([]float64, nSlots)
		}
		copy(loopArgs[:nLoopBinds], initVals)
		for ci, cap := range caps {
			if cap.isDynamic {
				loopArgs[nLoopBinds+ci] = fnArgs[cap.argIdx]
			} else {
				loopArgs[nLoopBinds+ci] = cap.constVal
			}
		}
		return loopNative(loopArgs)
	}
}

// ir_call_dispatch.go — IR-aware function call dispatch for the tree-walker.
//
// When the tree-walker (CallExpr.Eval) calls a *Fn, this tries to dispatch
// through the IR executor (typed then boxed) before falling back to fn.Call.
// This eliminates environment frame allocation and enables irValue-based
// execution for compiled functions called from non-IR code paths.

func irDispatchFnCall(fn *Fn, args []coretypes.Object) coretypes.Object {
	// Only try IR dispatch for self-recursive fns (proven correct patterns)
	// and fns with native helpers. Other fns may have subtle correctness
	// differences between IR and tree-walker evaluation.
	if fnProg := irCompileFn(fn); fnProg != nil && (fnProg.hasSelf || runtimeExec.HasNativeHelper(fnProg)) {
		var result coretypes.Object
		func() {
			defer func() {
				if r := recover(); r != nil {
					result = nil
				}
			}()
			result = irExecTyped(fnProg, args)
			if result == nil {
				result = irExec(fnProg, args)
			}
		}()
		if result != nil {
			return result
		}
	}
	if len(args) > 0 {
		// CallExpr's fixed-arity fast paths pass slices backed by stack arrays.
		// fn.Call stores the argument slice in the lexical environment, so a
		// closure returned from that call must not retain stack-backed storage
		// that a later call can overwrite.
		args = coreir.StableArgs(args)
	}
	return fn.Call(args)
}

// ---- fn_ir_compile.go ----
// ---------- Fn compilation ----------

// irCompileFn attempts to compile a single-arity Fn body into an IRProgram.
// The args become slots 0..n-1. Returns nil if the body can't be compiled.
// selfBinding optionally identifies the binding key for self-recursive calls.
func irCompileFn(fn *Fn) *IRProgram {
	// Variadic-only fn (fn [x & rest] ...)
	if len(fn.fnExpr.arities) == 0 && fn.fnExpr.variadic != nil {
		return irCompileVariadicFn(fn)
	}
	if len(fn.fnExpr.arities) == 0 {
		return nil
	}
	// Single arity: original fast path
	if len(fn.fnExpr.arities) == 1 && fn.fnExpr.variadic == nil {
		arity := fn.fnExpr.arities[0]
		return irCompileSingleArity(fn, arity)
	}
	// Multi-arity: compile each arity separately
	return irCompileMultiArity(fn)
}

func irCompileSingleArity(fn *Fn, arity FnArityExpr) *IRProgram {
	arityKey := &fn.fnExpr.arities[0]
	if cached, ok := irFnCache.Load(arityKey); ok {
		prog := cached.(*IRProgram)
		if prog == irCompileFailed {
			return nil
		}
		return prog
	}

	// Determine the frame from the body
	fnFrame := guessLoopFrame(arity.body)
	if fnFrame < 0 {
		fnFrame = guessFnParamFrame(arity.body, len(arity.args))
	}
	if fnFrame < 0 {
		fnFrame = 1
	}
	// Try compilation with guessed frame; if it fails, retry with frame+1
	// (the guess can pick a capture frame instead of the param frame)
	for attempt := 0; attempt < 2; attempt++ {
		trialFrame := fnFrame + attempt
		prog := irCompileFnWithFrame(fn, arity, trialFrame)
		if prog != nil {
			irFnCache.Store(arityKey, prog)
			return prog
		}
	}
	irFnCache.Store(arityKey, irCompileFailed)
	return nil
}

func irCompileFnWithFrame(fn *Fn, arity FnArityExpr, fnFrame int) *IRProgram {
	// Auto-detect frame if -1
	if fnFrame < 0 {
		fnFrame = guessLoopFrame(arity.body)
		if fnFrame < 0 {
			fnFrame = guessFnParamFrame(arity.body, len(arity.args))
		}
		if fnFrame < 0 {
			fnFrame = 1
		}
	}
	c := &irCompiler{
		bindingMap: make(map[bindingKey]int),
		loopFrame:  -1,
	}
	c.numSlots = len(arity.args)
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
	if fn.fnExpr.self.NameKey() != nil {
		// The self-binding is typically at frame fnFrame-1, index 0
		// (the letfn/fn frame that holds the fn itself)
		c.selfSlot = 0 // will use special irCallSelf opcode
		c.hasSelf = true
		c.selfNArgs = len(arity.args)
	}

	// If the fn was defined via defn, enable var-based self-call detection
	if fn.defVar != nil {
		c.hasSelf = true
		c.selfVar = fn.defVar
		c.selfNArgs = len(arity.args)
	}

	// Compile body
	for i, expr := range arity.body {
		if !c.compileExpr(expr, i == len(arity.body)-1) {
			return nil
		}
	}
	if len(c.code) == 0 {

		return nil
	}
	if c.code[len(c.code)-1] != irReturn {
		c.emit(irReturn)
	}
	// Compute capture slot indices (where each capture goes in the slot array).
	// Actual capture VALUES are resolved dynamically at call time from fn.env.
	if len(c.captureKeys) > 0 {
		capIdxs := make([]int, len(c.captureKeys))
		for ci, ck := range c.captureKeys {
			capIdxs[ci] = c.bindingMap[ck]
		}
		c.captureSlotIdxs = capIdxs
	}
	prog := &IRProgram{
		code:            c.code,
		constants:       c.constants,
		numSlots:        c.numSlots,
		captureKeys:     c.captureKeys,
		captureSlots:    c.captureSlots,
		captureSlotIdxs: c.captureSlotIdxs,
		hasSelf:         c.hasSelf,
		fnExprs:         c.fnExprs,
		traceName:       fn.fnExpr.traceName,
	}
	// Eagerly compile native f64 helper if eligible
	// Pre-compute capture slot set for fast irCallSelf
	if len(c.captureSlotIdxs) > 0 && c.hasSelf {
		prog.captureSlotSet = make([]bool, c.numSlots)
		for _, idx := range c.captureSlotIdxs {
			prog.captureSlotSet[idx] = true
		}
	}
	prog.refreshModel()
	runtimeExec.InstallNativeHelper(prog, irCompileNativeHelper(prog))
	return prog
}

// irCompileMultiArity compiles a multi-arity fn into an IRProgram with
// arityPrograms map for dispatch by arg count.
func irCompileMultiArity(fn *Fn) *IRProgram {
	// Check cache using first arity
	firstArityKey := &fn.fnExpr.arities[0]
	if cached, ok := irFnCache.Load(firstArityKey); ok {
		prog := cached.(*IRProgram)
		if prog == irCompileFailed {
			return nil
		}
		return prog
	}

	programs := make(map[int]*IRProgram)
	for _, arity := range fn.fnExpr.arities {
		prog := irCompileFnWithFrame(fn, arity, -1) // -1 means auto-detect
		if prog == nil {
			// If any arity fails, mark the whole fn as failed
			irFnCache.Store(firstArityKey, irCompileFailed)
			return nil
		}
		programs[len(arity.args)] = prog
	}

	// Handle variadic arity
	var varProg *IRProgram
	varMinArgs := 0
	if fn.fnExpr.variadic != nil {
		va := *fn.fnExpr.variadic
		varProg = irCompileFnWithFrame(fn, va, -1)
		if varProg != nil {
			varMinArgs = len(va.args)
		}
	}

	// Create wrapper program that dispatches by arity
	wrapper := (&IRProgram{
		arityPrograms:   programs,
		variadicProg:    varProg,
		variadicMinArgs: varMinArgs,
		traceName:       fn.fnExpr.traceName,
	}).refreshModel()
	irFnCache.Store(firstArityKey, wrapper)
	return wrapper
}

// irCompileVariadicFn compiles a variadic fn (fn [x & rest] ...).
// The rest parameter is packed into a vector from remaining args.
func irCompileVariadicFn(fn *Fn) *IRProgram {
	va := *fn.fnExpr.variadic
	variadicKey := fn.fnExpr.variadic

	if cached, ok := irFnCache.Load(variadicKey); ok {
		prog := cached.(*IRProgram)
		if prog == irCompileFailed {
			return nil
		}
		return prog
	}

	// The variadic arity has named args + one rest arg.
	// args slice passed to the fn has arbitrary length >= len(va.args)-1
	// (the last arg in va.args is the rest parameter).
	// We compile the body with all named args as slots, plus the rest slot.
	prog := irCompileFnWithFrame(fn, va, -1)
	if prog == nil || len(prog.captureKeys) > 0 {
		// Variadic functions with closed-over bindings need exact rest-slot
		// frame handling. Keep them on the tree-walker until the IR variadic
		// closure path is capture-safe; otherwise forms like (constantly x)
		// can read the packed rest argument instead of the captured value.
		irFnCache.Store(variadicKey, irCompileFailed)
		return nil
	}
	// Mark as variadic so the executor knows to pack rest args
	prog.variadicMinArgs = len(va.args) - 1 // exclude the & rest param from required count
	prog.refreshModel()
	irFnCache.Store(variadicKey, prog)
	return prog
}

// ---- program_envelope.go ----
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
	irToTransient    = coreir.ToTransient    // pop 1 (corecollections.ArrayVector), push TransientVector
	irAssocBang      = coreir.AssocBang      // pop 3 (tv, key, val), mutate in place, push tv
	irToPersistent   = coreir.ToPersistent   // pop 1 (TransientVector), push corecollections.ArrayVector
	irFallback       = coreir.Fallback       // cannot execute in IR; fall back to tree Eval
	irIntCast        = coreir.IntCast        // pop 1 (Char or coretypes.Number), push Int
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

// AnalyzeIRProgram returns a conservative program-shape summary for diagnostics
// and optimization gates.
func AnalyzeIRProgram(prog *IRProgram) coreir.Analysis {
	if prog == nil {
		return coreir.Analysis{SuggestedPath: "none"}
	}
	if prog.analysis != nil {
		return *prog.analysis
	}
	info := prog.escapeInfo
	if info == nil {
		info = analyzeEscapes(prog)
		prog.escapeInfo = info
	}
	model := prog.neutralModel()
	a := coreir.Analyze(
		model.Code,
		model.NumSlots,
		len(prog.captureKeys),
		corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0),
		info.StringBuilderSlots,
		info.StringPrependSlots,
	)
	prog.analysis = &a
	model.Analysis = &a
	return a
}

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
	constants       []coretypes.Object
	numSlots        int
	captureKeys     []bindingKey
	captureSlots    []coretypes.Object // resolved capture values from fn.env
	captureSlotIdxs []int              // slot indices for each capture
	hasSelf         bool
	escapeInfo      *EscapeInfo
	analysis        *coreir.Analysis
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

func (p *IRProgram) neutralFloatConsts() []float64 {
	if p == nil {
		return nil
	}
	if len(p.floatConsts) > 0 {
		return append([]float64(nil), p.floatConsts...)
	}
	var floats []float64
	for _, constant := range p.constants {
		if v, ok := constant.(coretypes.Double); ok {
			floats = append(floats, v.D)
		}
	}
	return floats
}

func (p *IRProgram) refreshModel() *IRProgram {
	if p == nil {
		return nil
	}
	model := coreir.NewProgram(p.code, p.numSlots, len(p.constants))
	model.HasSelf = p.hasSelf
	model.FloatConsts = p.neutralFloatConsts()
	model.WithCaptures(p.captureSlotIdxs, p.captureSlotSet)
	if p.analysis != nil {
		analysis := coreir.Analyze(p.code, p.numSlots, len(p.captureKeys), corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0), p.analysis.StringAppendSlots, p.analysis.StringPrependSlots)
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

// ---- escape_analysis.go ----
// escape_analysis.go — determines which IR slots can safely use in-place mutation.
//
// A slot is "non-escaping" if:
// 1. It is only read via irLoadSlot
// 2. It is only written via irStoreSlot or irRecur
// 3. It is only consumed by irAssoc/irNth/irGet/irGet3 (collection ops that
//    produce new values without retaining references to the original)
// 4. It is NOT passed to irCallSlot/irCallSelf (which could alias it)
//
// Non-escaping vector slots can use in-place mutation (transient optimization).

// EscapeInfo holds escape analysis results for an IR program.
type EscapeInfo struct {
	// SafeMutableSlots[i] = true means slot i can use transient builders.
	SafeMutableSlots []bool
	// StringBuilderSlots[i] = true means slot i is used as the left operand
	// of irStr2 and can benefit from append-style TransientString building.
	StringBuilderSlots []bool
	// StringPrependSlots[i] = true means slot i is used as the right operand
	// of irStr2 and can benefit from prepend-style TransientString building.
	StringPrependSlots []bool
}

// analyzeEscapes performs escape analysis on an IR program.
func analyzeEscapes(prog *IRProgram) *EscapeInfo {
	info := &EscapeInfo{
		SafeMutableSlots:   make([]bool, prog.numSlots),
		StringBuilderSlots: make([]bool, prog.numSlots),
		StringPrependSlots: make([]bool, prog.numSlots),
	}

	// Start by assuming all slots are safe
	for i := range info.SafeMutableSlots {
		info.SafeMutableSlots[i] = true
	}

	code := prog.code
	pc := 0

	// Track which slots are used as arguments to function calls
	// or other operations that could retain references.
	//
	// Strategy: walk the bytecode and track the stack symbolically.
	// When a slot value reaches a call argument position, mark it unsafe.

	type stackEntry struct {
		fromSlot int // which slot this value came from, or -1
	}

	stack := make([]stackEntry, 0, 16)

	push := func(slot int) {
		stack = append(stack, stackEntry{fromSlot: slot})
	}
	pop := func() stackEntry {
		if len(stack) == 0 {
			return stackEntry{fromSlot: -1}
		}
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return e
	}
	popN := func(n int) []stackEntry {
		entries := make([]stackEntry, n)
		for i := n - 1; i >= 0; i-- {
			entries[i] = pop()
		}
		return entries
	}

	for pc < len(code) {
		op := code[pc]
		pc++

		switch op {
		case irLiteral:
			pc += 2
			push(-1) // literal, not from a slot

		case irNthStringASCII:
			pc += 2
			pop() // index
			push(-1)

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			push(idx)

		case irStoreSlot:
			pc += 2
			pop()

		case irAdd, irSub, irMul, irDiv, irRem, irLt, irGte, irGt, irLte, irEq:
			pop()
			pop()
			push(-1) // result not from a slot

		case irInc, irDec, irIsZero, irSqrt, irFirst, irCount:
			pop()
			push(-1)

		case irGet:
			// get(coll, key) — coll is consumed, doesn't escape
			pop() // key
			pop() // coll — used for lookup only, safe
			push(-1)

		case irGet3:
			pop() // default
			pop() // key
			pop() // coll — safe
			push(-1)

		case irAssoc:
			// assoc(coll, key, val) stores key/val in the resulting collection.
			// The collection slot itself remains safe for transient mutation, but
			// key/value slots escape into the collection and must not be mutable
			// builders (e.g. TransientString).
			val := pop()
			key := pop()
			pop() // coll — safe
			if val.fromSlot >= 0 {
				info.SafeMutableSlots[val.fromSlot] = false
			}
			if key.fromSlot >= 0 {
				info.SafeMutableSlots[key.fromSlot] = false
			}
			push(-1)

		case irNth:
			pop() // idx
			pop() // coll — safe
			push(-1)

		case irConj:
			pop() // val
			pop() // coll — safe
			push(-1)

		case irCallSlot:
			pc += 4 // slot(2) + nargs(2)
			nargs := int(code[pc-2])<<8 | int(code[pc-1])
			// All arguments to a call ESCAPE — the function could retain them
			args := popN(nargs)
			for _, a := range args {
				if a.fromSlot >= 0 {
					info.SafeMutableSlots[a.fromSlot] = false
				}
			}
			push(-1) // result

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Self-call args also escape
			args := popN(nargs)
			for _, a := range args {
				if a.fromSlot >= 0 {
					info.SafeMutableSlots[a.fromSlot] = false
				}
			}
			push(-1)

		case irJumpIfNot:
			pc += 2
			pop() // condition consumed

		case irJump:
			pc += 2

		case irReturn:
			// Return value doesn't affect in-function mutation safety.
			_ = pop()

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			targetPC := int(code[pc-2])<<8 | int(code[pc-1])
			if targetPC != 0 {
				pc += 2
			}
			// Recur rebinds slots — this is safe (same as initial binding)
			popN(nargs)

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			popN(n)
			push(-1)

		case irStr1:
			pop()
			push(-1)

		case irStr2:
			b := pop()
			a := pop()
			if a.fromSlot >= 0 {
				info.StringBuilderSlots[a.fromSlot] = true
			}
			if b.fromSlot >= 0 {
				info.StringPrependSlots[b.fromSlot] = true
			}
			push(-1)

		case irToTransient, irToPersistent, irAssocBang:
			// These shouldn't appear in programs being analyzed
			// (they're generated by the optimization itself)
			pop()
			push(-1)

		default:
			// Unknown op — conservatively mark all slots as unsafe
			for i := range info.SafeMutableSlots {
				info.SafeMutableSlots[i] = false
			}
			return info
		}
	}

	return info
}

// ---- runtime_ir_exports.go ----
// ir_exported.go — exported functions for the joker.runtime namespace.
// These bridge internal IR/WASM/escape analysis to the public API.

// IrDisassemble returns a human-readable disassembly of an IR program.
func IrDisassemble(prog *IRProgram) string {
	if prog == nil {
		return "; nil program"
	}
	model := prog.neutralModel()
	return coreir.Disassemble(model.Code, func(idx int) string {
		if idx < len(prog.constants) && prog.constants[idx] != nil {
			return prog.constants[idx].ToString(false)
		}
		return ""
	})
}

// ExplainWASMEligibility exposes the WASM diagnostic for a program.
func ExplainWASMEligibility(prog *IRProgram) corewasm.Diagnostic {
	return explainWASMEligibility(prog)
}

// AnalyzeEscapesExported returns the safe-mutable-slots boolean array.
func AnalyzeEscapesExported(prog *IRProgram) []bool {
	info := analyzeEscapes(prog)
	return info.SafeMutableSlots
}

// IRProgram accessor methods for external packages.
func (p *IRProgram) CodeLen() int {
	model := p.neutralModel()
	return len(model.Code)
}

func (p *IRProgram) CodeBytes() []byte {
	model := p.neutralModel()
	return append([]byte(nil), model.Code...)
}

func (p *IRProgram) ConstLen() int { return len(p.constants) }
func (p *IRProgram) Constants() []coretypes.Object {
	return append([]coretypes.Object(nil), p.constants...)
}
func (p *IRProgram) NumSlots() int {
	model := p.neutralModel()
	return model.NumSlots
}
func (p *IRProgram) HasSelf() bool                    { return p.hasSelf }
func (p *IRProgram) CaptureSlots() []coretypes.Object { return p.captureSlots }
func (p *IRProgram) GetNativeHelper() func([]float64) float64 {
	if nativeHelper, ok := runtimeExec.NativeHelper(p); ok {
		return func(args []float64) float64 { return nativeHelper(args) }
	}
	return nil
}

// Exports for std/jit and std/runtime namespaces.
func IrCompileFn(fn *Fn) *IRProgram                                      { return irCompileFn(fn) }
func IrExecTyped(prog *IRProgram, s []coretypes.Object) coretypes.Object { return irExecTyped(prog, s) }
func IrExec(prog *IRProgram, s []coretypes.Object) coretypes.Object      { return irExec(prog, s) }

func IsFloatExported(prog *IRProgram) bool {
	model := prog.neutralModel()
	return model != nil && corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0)
}

func IrToWasmExported(prog *IRProgram) []byte { return irToWasm(prog) }

func WasmCompileBytesExported(prog *IRProgram) []byte {
	wp := wasmCompile(prog)
	if wp == nil {
		return nil
	}
	return append([]byte(nil), wp.bytes...)
}

type IRAnalysisExported struct {
	Eligible       bool
	Path           string
	HasCallSlot    bool
	HasSelfCall    bool
	UsesCollection bool
	UsesString     bool
	HasMapOps      bool
	HasAssoc       bool
	HasGenericNth  bool
}

func AnalyzeIRProgramExported(prog *IRProgram) IRAnalysisExported {
	a := AnalyzeIRProgram(prog)
	return IRAnalysisExported{
		Eligible:       irTypedEligible(a),
		Path:           a.SuggestedPath,
		HasCallSlot:    a.HasCallSlot,
		HasSelfCall:    a.HasSelfCall,
		UsesCollection: a.UsesCollection,
		UsesString:     a.UsesString,
		HasMapOps:      a.HasMapOps,
		HasAssoc:       a.HasAssoc,
		HasGenericNth:  a.HasGenericNth,
	}
}

// ---- native_recursive.go ----
// native_recursive.go — Native Go code generation for pure-integer recursive fns.
//
// When a fn body contains only integer arithmetic, comparisons, and self-recursive
// calls (no collections, strings, or other types), we compile to fixed-arity
// native Go functions that run without coretypes.Object boxing, interface dispatch, or
// slice allocation per call.

// nativeIntFn1 through nativeIntFn3 are typed native function signatures.
type nativeIntFn1 func(a int) int
type nativeIntFn2 func(a, b int) int
type nativeIntFn3 func(a, b, c int) int

// nativeRecursiveEntry holds a compiled native fn for a specific arity.
type nativeRecursiveEntry struct {
	arity int
	fn1   nativeIntFn1
	fn2   nativeIntFn2
	fn3   nativeIntFn3
}

var nativeRecursiveCache sync.Map // *Fn → *nativeRecursiveEntry (or nativeRecursiveFailed sentinel)
var nativeRecursiveFailed = &nativeRecursiveEntry{arity: -1}

func tryNativeRecursive(fn *Fn) *nativeRecursiveEntry {
	if cached, ok := nativeRecursiveCache.Load(fn); ok {
		entry := cached.(*nativeRecursiveEntry)
		if entry == nativeRecursiveFailed {
			return nil
		}
		return entry
	}

	entry := compileNativeRecursive(fn)
	if entry == nil {
		nativeRecursiveCache.Store(fn, nativeRecursiveFailed)
	} else {
		nativeRecursiveCache.Store(fn, entry)
	}
	return entry
}

func compileNativeRecursive(fn *Fn) *nativeRecursiveEntry {
	if fn == nil || fn.fnExpr == nil || fn.defVar == nil {
		return nil
	}
	if len(fn.fnExpr.arities) != 1 || fn.fnExpr.variadic != nil {
		return nil
	}
	arity := fn.fnExpr.arities[0]
	nargs := len(arity.args)
	if nargs < 1 || nargs > 3 || len(arity.body) != 1 {
		return nil
	}

	selfVar := fn.defVar
	paramFrame := guessFnParamFrame(arity.body, nargs)
	if paramFrame < 0 {
		paramFrame = 1
	}

	entry := &nativeRecursiveEntry{arity: nargs}

	switch nargs {
	case 1:
		compiled := compileIntExpr1(arity.body[0], selfVar, paramFrame, entry)
		if compiled == nil {
			return nil
		}
		entry.fn1 = compiled
	case 2:
		compiled := compileIntExpr2(arity.body[0], selfVar, paramFrame, entry)
		if compiled == nil {
			return nil
		}
		entry.fn2 = compiled
	case 3:
		compiled := compileIntExpr3(arity.body[0], selfVar, paramFrame, entry)
		if compiled == nil {
			return nil
		}
		entry.fn3 = compiled
	}
	return entry
}

func callNativeRecursive(entry *nativeRecursiveEntry, args []coretypes.Object) coretypes.Object {
	switch entry.arity {
	case 1:
		a, ok := args[0].(coretypes.Int)
		if !ok {
			return nil
		}
		return coretypes.Int{I: entry.fn1(a.I)}
	case 2:
		a, aok := args[0].(coretypes.Int)
		b, bok := args[1].(coretypes.Int)
		if !aok || !bok {
			return nil
		}
		return coretypes.Int{I: entry.fn2(a.I, b.I)}
	case 3:
		a, aok := args[0].(coretypes.Int)
		b, bok := args[1].(coretypes.Int)
		c, cok := args[2].(coretypes.Int)
		if !aok || !bok || !cok {
			return nil
		}
		return coretypes.Int{I: entry.fn3(a.I, b.I, c.I)}
	}
	return nil
}

// --- Arity-1 compiler (fib) ---

type intBool1 func(a int) bool

func compileIntExpr1(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn1 {
	switch e := expr.(type) {
	case *LiteralExpr:
		if v, ok := e.obj.(coretypes.Int); ok {
			val := v.I
			return func(a int) int { return val }
		}
	case *BindingExpr:
		if e.binding.frame == pf && e.binding.index == 0 {
			return func(a int) int { return a }
		}
	case *IfExpr:
		cond := compileIntBool1(e.cond, selfVar, pf, entry)
		pos := compileIntExpr1(e.positive, selfVar, pf, entry)
		neg := compileIntExpr1(e.negative, selfVar, pf, entry)
		if cond == nil || pos == nil || neg == nil {
			return nil
		}
		return func(a int) int {
			if cond(a) {
				return pos(a)
			}
			return neg(a)
		}
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok && vref.vr == selfVar && len(e.args) == 1 {
			arg := compileIntExpr1(e.args[0], selfVar, pf, entry)
			if arg == nil {
				return nil
			}
			return func(a int) int { return entry.fn1(arg(a)) }
		}
		if vref, ok := e.callable.(*VarRefExpr); ok {
			return compileArith1(coreVarToProcName(vref.vr), e.args, selfVar, pf, entry)
		}
	}
	return nil
}

func compileIntBool1(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) intBool1 {
	e, ok := expr.(*CallExpr)
	if !ok {
		return nil
	}
	vref, ok := e.callable.(*VarRefExpr)
	if !ok || len(e.args) != 2 {
		return nil
	}
	a := compileIntExpr1(e.args[0], selfVar, pf, entry)
	b := compileIntExpr1(e.args[1], selfVar, pf, entry)
	if a == nil || b == nil {
		return nil
	}
	switch coreVarToProcName(vref.vr) {
	case "procLt":
		return func(x int) bool { return a(x) < b(x) }
	case "procLte":
		return func(x int) bool { return a(x) <= b(x) }
	case "procGt":
		return func(x int) bool { return a(x) > b(x) }
	case "procGte":
		return func(x int) bool { return a(x) >= b(x) }
	case "procEq":
		return func(x int) bool { return a(x) == b(x) }
	}
	return nil
}

func compileArith1(proc string, args []Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn1 {
	switch proc {
	case "procAdd":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		b := compileIntExpr1(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x int) int { return a(x) + b(x) }
	case "procSubtract":
		if len(args) == 1 {
			a := compileIntExpr1(args[0], selfVar, pf, entry)
			if a == nil {
				return nil
			}
			return func(x int) int { return -a(x) }
		}
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		b := compileIntExpr1(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x int) int { return a(x) - b(x) }
	case "procMultiply":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		b := compileIntExpr1(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x int) int { return a(x) * b(x) }
	case "procInc":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x int) int { return a(x) + 1 }
	case "procDec":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x int) int { return a(x) - 1 }
	}
	return nil
}

// --- Arity-3 compiler (tak) ---

type intBool3 func(a, b, c int) bool

func compileIntExpr3(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn3 {
	switch e := expr.(type) {
	case *LiteralExpr:
		if v, ok := e.obj.(coretypes.Int); ok {
			val := v.I
			return func(a, b, c int) int { return val }
		}
	case *BindingExpr:
		if e.binding.frame == pf {
			switch e.binding.index {
			case 0:
				return func(a, b, c int) int { return a }
			case 1:
				return func(a, b, c int) int { return b }
			case 2:
				return func(a, b, c int) int { return c }
			}
		}
	case *IfExpr:
		cond := compileIntBool3(e.cond, selfVar, pf, entry)
		pos := compileIntExpr3(e.positive, selfVar, pf, entry)
		neg := compileIntExpr3(e.negative, selfVar, pf, entry)
		if cond == nil || pos == nil || neg == nil {
			return nil
		}
		return func(a, b, c int) int {
			if cond(a, b, c) {
				return pos(a, b, c)
			}
			return neg(a, b, c)
		}
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok && vref.vr == selfVar && len(e.args) == 3 {
			x := compileIntExpr3(e.args[0], selfVar, pf, entry)
			y := compileIntExpr3(e.args[1], selfVar, pf, entry)
			z := compileIntExpr3(e.args[2], selfVar, pf, entry)
			if x == nil || y == nil || z == nil {
				return nil
			}
			return func(a, b, c int) int { return entry.fn3(x(a, b, c), y(a, b, c), z(a, b, c)) }
		}
		if vref, ok := e.callable.(*VarRefExpr); ok {
			return compileArith3(coreVarToProcName(vref.vr), e.args, selfVar, pf, entry)
		}
	}
	return nil
}

func compileIntBool3(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) intBool3 {
	e, ok := expr.(*CallExpr)
	if !ok {
		return nil
	}
	vref, ok := e.callable.(*VarRefExpr)
	if !ok || len(e.args) != 2 {
		return nil
	}
	a := compileIntExpr3(e.args[0], selfVar, pf, entry)
	b := compileIntExpr3(e.args[1], selfVar, pf, entry)
	if a == nil || b == nil {
		return nil
	}
	switch coreVarToProcName(vref.vr) {
	case "procLt":
		return func(x, y, z int) bool { return a(x, y, z) < b(x, y, z) }
	case "procLte":
		return func(x, y, z int) bool { return a(x, y, z) <= b(x, y, z) }
	case "procGt":
		return func(x, y, z int) bool { return a(x, y, z) > b(x, y, z) }
	case "procGte":
		return func(x, y, z int) bool { return a(x, y, z) >= b(x, y, z) }
	case "procEq":
		return func(x, y, z int) bool { return a(x, y, z) == b(x, y, z) }
	}
	return nil
}

func compileArith3(proc string, args []Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn3 {
	switch proc {
	case "procAdd":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		b := compileIntExpr3(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y, z int) int { return a(x, y, z) + b(x, y, z) }
	case "procSubtract":
		if len(args) == 1 {
			a := compileIntExpr3(args[0], selfVar, pf, entry)
			if a == nil {
				return nil
			}
			return func(x, y, z int) int { return -a(x, y, z) }
		}
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		b := compileIntExpr3(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y, z int) int { return a(x, y, z) - b(x, y, z) }
	case "procMultiply":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		b := compileIntExpr3(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y, z int) int { return a(x, y, z) * b(x, y, z) }
	case "procInc":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y, z int) int { return a(x, y, z) + 1 }
	case "procDec":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y, z int) int { return a(x, y, z) - 1 }
	}
	return nil
}

// --- Arity-2 compiler ---

type intBool2 func(a, b int) bool

func compileIntExpr2(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn2 {
	switch e := expr.(type) {
	case *LiteralExpr:
		if v, ok := e.obj.(coretypes.Int); ok {
			val := v.I
			return func(a, b int) int { return val }
		}
	case *BindingExpr:
		if e.binding.frame == pf {
			switch e.binding.index {
			case 0:
				return func(a, b int) int { return a }
			case 1:
				return func(a, b int) int { return b }
			}
		}
	case *IfExpr:
		cond := compileIntBool2(e.cond, selfVar, pf, entry)
		pos := compileIntExpr2(e.positive, selfVar, pf, entry)
		neg := compileIntExpr2(e.negative, selfVar, pf, entry)
		if cond == nil || pos == nil || neg == nil {
			return nil
		}
		return func(a, b int) int {
			if cond(a, b) {
				return pos(a, b)
			}
			return neg(a, b)
		}
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok && vref.vr == selfVar && len(e.args) == 2 {
			x := compileIntExpr2(e.args[0], selfVar, pf, entry)
			y := compileIntExpr2(e.args[1], selfVar, pf, entry)
			if x == nil || y == nil {
				return nil
			}
			return func(a, b int) int { return entry.fn2(x(a, b), y(a, b)) }
		}
		if vref, ok := e.callable.(*VarRefExpr); ok {
			return compileArith2(coreVarToProcName(vref.vr), e.args, selfVar, pf, entry)
		}
	}
	return nil
}

func compileIntBool2(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) intBool2 {
	e, ok := expr.(*CallExpr)
	if !ok {
		return nil
	}
	vref, ok := e.callable.(*VarRefExpr)
	if !ok || len(e.args) != 2 {
		return nil
	}
	a := compileIntExpr2(e.args[0], selfVar, pf, entry)
	b := compileIntExpr2(e.args[1], selfVar, pf, entry)
	if a == nil || b == nil {
		return nil
	}
	switch coreVarToProcName(vref.vr) {
	case "procLt":
		return func(x, y int) bool { return a(x, y) < b(x, y) }
	case "procLte":
		return func(x, y int) bool { return a(x, y) <= b(x, y) }
	case "procGt":
		return func(x, y int) bool { return a(x, y) > b(x, y) }
	case "procGte":
		return func(x, y int) bool { return a(x, y) >= b(x, y) }
	case "procEq":
		return func(x, y int) bool { return a(x, y) == b(x, y) }
	}
	return nil
}

func compileArith2(proc string, args []Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn2 {
	switch proc {
	case "procAdd":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		b := compileIntExpr2(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y int) int { return a(x, y) + b(x, y) }
	case "procSubtract":
		if len(args) == 1 {
			a := compileIntExpr2(args[0], selfVar, pf, entry)
			if a == nil {
				return nil
			}
			return func(x, y int) int { return -a(x, y) }
		}
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		b := compileIntExpr2(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y int) int { return a(x, y) - b(x, y) }
	case "procMultiply":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		b := compileIntExpr2(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y int) int { return a(x, y) * b(x, y) }
	case "procInc":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y int) int { return a(x, y) + 1 }
	case "procDec":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y int) int { return a(x, y) - 1 }
	}
	return nil
}

// ---- loop_compiler.go ----

// ---- loop_compiler.go ----
// ---------- Compiler ----------

type bindingKey struct {
	frame int
	index int
}

type irCompiler struct {
	code             []byte
	constants        []coretypes.Object
	bindingMap       map[bindingKey]int
	captureKeys      []bindingKey
	captureSlots     []coretypes.Object
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

func (c *irCompiler) addConstant(obj coretypes.Object) int {
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
		if s, ok := e.obj.(coretypes.String); ok && isASCIIBytes(s.S) {
			return s.S, true
		}
	case *BindingExpr:
		if lit, ok := e.binding.value.(*LiteralExpr); ok {
			if s, ok := lit.obj.(coretypes.String); ok && isASCIIBytes(s.S) {
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
		case coretypes.String:
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
				case coretypes.String:
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
			arr := make([]coretypes.Object, len(e.v))
			for i, elem := range e.v {
				arr[i] = elem.(*LiteralExpr).obj
			}
			idx := c.addConstant(&corecollections.ArrayVector{Arr: arr})
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
		var obj coretypes.Object
		if int64(len(e.keys)) > corecollections.HASHMAP_THRESHOLD/2 {
			res := corecollections.EmptyHashMap
			for i := range e.keys {
				key := e.keys[i].(*LiteralExpr).obj
				if res.ContainsKey(key) {
					return c.reject("duplicate key in IR map literal: %s", key.ToString(false))
				}
				res = res.Assoc(key, e.values[i].(*LiteralExpr).obj).(*corecollections.HashMap)
			}
			obj = res
		} else {
			res := corecollections.EmptyArrayMap()
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
		case coretypes.String, coretypes.Char:
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
		case coretypes.Int, coretypes.Double:
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
// float64 values directly, eliminating WASM/IR dispatch and coretypes.Object boxing.

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
		case coretypes.Int:
			consts[i] = float64(v.I)
		case coretypes.Double:
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
			idx := c.addConstant(coretypes.String{S: s})
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
			idx := c.addConstant(coretypes.Int{I: n})
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

// ---- wasm_compile.go ----

// ---- wasm_compile.go ----
// wasm_codegen.go — translates IR bytecode to WASM function body.

var wasmFnCache sync.Map // map[*FnArityExpr]*WasmProgram

var wasmFnFail = &WasmProgram{}

// wasmGetFn retrieves or compiles a WASM program for a Fn.
func wasmGetFn(fn *Fn) *WasmProgram {
	if len(fn.fnExpr.arities) != 1 {
		return nil
	}
	arity := &fn.fnExpr.arities[0]

	if v, ok := wasmFnCache.Load(arity); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFnFail {
			return nil
		}
		return wp
	}

	// First compile to IR
	irProg := irCompileFn(fn)
	if irProg == nil {
		wasmFnCache.Store(arity, wasmFnFail)
		return nil
	}

	// Then try WASM
	wp := wasmCompile(irProg)
	if wp == nil {
		wasmFnCache.Store(arity, wasmFnFail)
		return nil
	}

	wasmFnCache.Store(arity, wp)
	return wp
}

func irToWasm(prog *IRProgram) []byte {
	model := prog.neutralModel()
	if model == nil || !corewasm.Eligible(model.Code) {
		return nil
	}
	useFloat := corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0)
	body := compileWasmBody(prog, useFloat)
	if body == nil {
		return nil
	}
	m := corewasm.NewModule()
	valType := corewasm.ValTypeI64
	if useFloat {
		valType = corewasm.ValTypeF64
	}
	m.AddTypeSectionTyped(model.NumSlots, valType)
	if prog.hasSelf {
		m.AddFuncSectionRecursive()
	} else {
		m.AddFuncSection()
	}
	m.AddExportSection()
	m.AddCodeSection(body)
	return m.Bytes()
}

// compileWasmBody generates WASM instructions.
//
// Layout:
//
//	block $exit (result i64)     ;; depth from inside if: +2
//	  loop $loop (void)          ;; depth from inside if: +1
//	    ;; body
//	    ;; irReturn → br $exit (depth = nesting + 1)
//	    ;; irRecur  → set locals, br $loop (depth = nesting)
//	  end
//	  i64.const 0  ;; unreachable
//	end
//
// For if/else: both branches end with `br` (stack-polymorphic),
// so `if void` works and no values need to flow through the if block.
func compileWasmBody(prog *IRProgram, useFloat bool) []byte {
	return compileWasmBodyWithHelper(prog, useFloat, -1, -1)
}

func compileWasmBodyWithHelper(prog *IRProgram, useFloat bool, helperSlot int, helperFuncIdx int) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	return compileWasmBodyWithHelperParams(prog, useFloat, helperSlot, helperFuncIdx, model.NumSlots)
}

func compileWasmBodyWithHelperParams(prog *IRProgram, useFloat bool, helperSlot int, helperFuncIdx int, numParams int) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	var o []byte
	valType := corewasm.ValTypeI64
	if useFloat {
		valType = corewasm.ValTypeF64
	}
	extraLocals := model.NumSlots - numParams
	if extraLocals > 0 {
		o = append(o, 0x01) // 1 local decl group
		o = corewasm.AppendULEB(o, extraLocals)
		o = append(o, valType)
	} else {
		o = append(o, 0x00) // 0 local decls
	}

	resType := valType
	if useFloat {
		resType = corewasm.ValTypeF64
	}
	o = append(o, 0x02, resType) // block $exit -> result type
	o = append(o, 0x03, 0x40)    // loop $loop -> void

	code := model.Code
	pc := 0
	depth := 0 // extra nesting from if blocks

	for pc < len(code) {
		op := code[pc]
		pc++

		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			c := prog.constants[idx]
			if useFloat {
				var fv float64
				switch v := c.(type) {
				case coretypes.Int:
					fv = float64(v.I)
				case coretypes.Double:
					fv = v.D
				default:
					return nil
				}
				o = append(o, 0x44) // f64.const
				o = corewasm.AppendF64(o, fv)
			} else {
				v, ok := c.(coretypes.Int)
				if !ok {
					return nil
				}
				o = append(o, 0x42) // i64.const
				o = corewasm.AppendSLEB(o, int64(v.I))
			}

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x20)
			o = corewasm.AppendULEB(o, idx)

		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x21)
			o = corewasm.AppendULEB(o, idx)

		case irAdd:
			if useFloat {
				o = append(o, 0xa0)
			} else {
				o = append(o, 0x7c)
			}
		case irSub:
			if useFloat {
				o = append(o, 0xa1)
			} else {
				o = append(o, 0x7d)
			}
		case irMul:
			if useFloat {
				o = append(o, 0xa2)
			} else {
				o = append(o, 0x7e)
			}
		case irDiv:
			if useFloat {
				o = append(o, 0xa3)
			} else {
				return nil
			}
		case irSqrt:
			if useFloat {
				o = append(o, 0x9f)
			} else {
				return nil
			}
		case irRem:
			if useFloat {
				return nil
			}
			o = append(o, 0x81)
		case irInc:
			if useFloat {
				o = append(o, 0x44)
				o = corewasm.AppendF64(o, 1.0)
				o = append(o, 0xa0)
			} else {
				o = append(o, 0x42, 0x01, 0x7c)
			}
		case irDec:
			if useFloat {
				o = append(o, 0x44)
				o = corewasm.AppendF64(o, 1.0)
				o = append(o, 0xa1)
			} else {
				o = append(o, 0x42, 0x01, 0x7d)
			}
		case irLt:
			if useFloat {
				o = append(o, 0x63) // f64.lt
			} else {
				o = append(o, 0x53, 0xad) // i64.lt_s, i64.extend_i32_s
			}
		case irGte:
			if useFloat {
				o = append(o, 0x65) // f64.ge
			} else {
				o = append(o, 0x56, 0xad) // i64.ge_s, i64.extend_i32_s
			}
		case irGt:
			if useFloat {
				o = append(o, 0x64) // f64.gt
			} else {
				o = append(o, 0x55, 0xad) // i64.gt_s, i64.extend_i32_s
			}
		case irLte:
			if useFloat {
				o = append(o, 0x66) // f64.le
			} else {
				o = append(o, 0x57, 0xad) // i64.le_s, i64.extend_i32_s
			}
		case irEq:
			if useFloat {
				o = append(o, 0x61)
			} else {
				o = append(o, 0x51, 0xad)
			}
		case irIsZero:
			if useFloat {
				o = append(o, 0x44)
				o = corewasm.AppendF64(o, 0.0)
				o = append(o, 0x61)
			} else {
				o = append(o, 0x50, 0xad)
			}

		case irJumpIfNot:
			pc += 2
			if !useFloat {
				o = append(o, 0xa7) // i32.wrap_i64
			}
			// In f64 mode, comparison already left i32 on stack
			o = append(o, 0x04, 0x40) // if void
			depth++

		case irJump:
			pc += 2
			o = append(o, 0x05) // else

		case irReturn:
			// Value on stack → br to $exit (block i64)
			// Depth: depth (ifs) + 1 (loop) + 0 ($exit is the block)
			// br N targets the Nth enclosing label from current position.
			// Labels: if_0..if_{depth-1}, loop, block
			// $exit = depth + 1
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth+1)
			// If we're inside an if and no explicit else follows,
			// emit else so the false branch code has somewhere to go.
			if depth > 0 && pc < len(code) && code[pc] != irJump {
				o = append(o, 0x05) // else
			}

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			for i := nargs - 1; i >= 0; i-- {
				o = append(o, 0x21)
				o = corewasm.AppendULEB(o, i)
			}
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth)
			pc = len(code)

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			_ = nargs // args already on WASM stack
			if slotIdx != helperSlot || helperFuncIdx < 0 {
				return nil
			}
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, helperFuncIdx)

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			_ = nargs                     // args already on WASM stack
			o = append(o, 0x10)           // call
			o = corewasm.AppendULEB(o, 0) // function index 0 (self)

		default:
			return nil
		}
	}

	// Close any open if blocks
	for depth > 0 {
		o = append(o, 0x0b)
		depth--
	}

	o = append(o, 0x0b) // end loop
	if useFloat {
		o = append(o, 0x44) // f64.const 0.0
		o = corewasm.AppendF64(o, 0.0)
	} else {
		o = append(o, 0x42, 0x00) // i64.const 0
	}
	o = append(o, 0x0b) // end block
	o = append(o, 0x0b) // end func
	return o
}

// ---- wasm_compile_host.go ----
// wasm_codegen_host.go — WASM codegen with host function imports.
//
// Extends the base codegen to emit modules that import the "joker"
// host module functions for collection operations. Programs with
// collection IR opcodes (irGet, irGet3, irAssoc, irNth, irConj, etc.)
// use this path instead of the pure-numeric codegen.

// standardHostImports lists the host functions in fixed order.
// Their indices in the WASM module are 0..len-1.
var standardHostImports = corewasm.StandardHostImports

// irToWasmWithImports compiles an IR program that uses collection ops
// to a WASM module with host function imports.
func irToWasmWithImports(prog *IRProgram) []byte {
	model := prog.neutralModel()
	if model == nil || !corewasm.EligibleWithImports(model.Code) {
		return nil
	}

	body := compileWasmBodyWithImports(prog)
	if body == nil {
		return nil
	}

	m := corewasm.NewModule()

	// Type section: one type per import + one for the main fn
	// All use i64 params and i64 result
	numTypes := len(standardHostImports) + 1
	var typeBody []byte
	typeBody = corewasm.AppendULEB(typeBody, numTypes)
	// Import function types (index 0..6)
	for _, imp := range standardHostImports {
		typeBody = append(typeBody, 0x60) // functype
		typeBody = corewasm.AppendULEB(typeBody, imp.NumParams)
		for j := 0; j < imp.NumParams; j++ {
			typeBody = append(typeBody, corewasm.ValTypeI64)
		}
		typeBody = append(typeBody, 0x01, corewasm.ValTypeI64)
	}
	// Main function type (index 7)
	typeBody = append(typeBody, 0x60)
	typeBody = corewasm.AppendULEB(typeBody, model.NumSlots)
	for i := 0; i < model.NumSlots; i++ {
		typeBody = append(typeBody, corewasm.ValTypeI64)
	}
	typeBody = append(typeBody, 0x01, corewasm.ValTypeI64)
	m.AddSection(0x01, typeBody)

	// Import section
	var importBody []byte
	importBody = corewasm.AppendULEB(importBody, len(standardHostImports))
	for i, imp := range standardHostImports {
		modName := []byte(wasmHostModuleName)
		importBody = corewasm.AppendULEB(importBody, len(modName))
		importBody = append(importBody, modName...)
		importBody = corewasm.AppendULEB(importBody, len(imp.Name))
		importBody = append(importBody, []byte(imp.Name)...)
		importBody = append(importBody, 0x00)           // import kind: func
		importBody = corewasm.AppendULEB(importBody, i) // type index
	}
	m.AddSection(0x02, importBody)

	// Function section: 1 function with type index = len(imports)
	mainTypeIdx := len(standardHostImports)
	var funcBody []byte
	funcBody = append(funcBody, 0x01)
	funcBody = corewasm.AppendULEB(funcBody, mainTypeIdx)
	m.AddSection(0x03, funcBody)

	// Export section: export the main function
	mainFuncIdx := len(standardHostImports) // imports are 0..6, main is 7
	name := []byte("exec")
	var exportBody []byte
	exportBody = append(exportBody, 0x01)
	exportBody = corewasm.AppendULEB(exportBody, len(name))
	exportBody = append(exportBody, name...)
	exportBody = append(exportBody, 0x00) // func export
	exportBody = corewasm.AppendULEB(exportBody, mainFuncIdx)
	m.AddSection(0x07, exportBody)

	// Code section
	m.AddCodeSection(body)

	return m.Bytes()
}

// compileWasmBodyWithImports generates function body with host call instructions.
func compileWasmBodyWithImports(prog *IRProgram) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	var o []byte
	o = append(o, 0x00) // 0 local decls

	o = append(o, 0x02, corewasm.ValTypeI64) // block $exit -> i64
	o = append(o, 0x03, 0x40)                // loop $loop -> void

	mainFuncIdx := len(standardHostImports)
	code := model.Code
	pc := 0
	depth := 0

	for pc < len(code) {
		op := code[pc]
		pc++

		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			c := prog.constants[idx]
			switch v := c.(type) {
			case coretypes.Int:
				o = append(o, 0x42)
				o = corewasm.AppendSLEB(o, int64(v.I))
			default:
				// Non-Int constant: use a pre-computed handle.
				// The handle value is: (1<<62) | constant_index
				// wasmExec will pre-populate the object table with these.
				handle := int64((1 << 62) | idx)
				o = append(o, 0x42)
				o = corewasm.AppendSLEB(o, handle)
			}

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x20)
			o = corewasm.AppendULEB(o, idx)

		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x21)
			o = corewasm.AppendULEB(o, idx)

		case irAdd:
			o = append(o, 0x7c)
		case irSub:
			o = append(o, 0x7d)
		case irMul:
			o = append(o, 0x7e)
		case irRem:
			o = append(o, 0x81)
		case irInc:
			o = append(o, 0x42, 0x01, 0x7c)
		case irDec:
			o = append(o, 0x42, 0x01, 0x7d)
		case irLt:
			o = append(o, 0x53, 0xad) // i64.lt_s, extend
		case irGte:
			o = append(o, 0x56, 0xad) // i64.ge_s, extend
		case irGt:
			o = append(o, 0x55, 0xad) // i64.gt_s, extend
		case irLte:
			o = append(o, 0x57, 0xad) // i64.le_s, extend
		case irEq:
			o = append(o, 0x51, 0xad)
		case irIsZero:
			o = append(o, 0x50, 0xad)

		// coretypes.Collection operations → call imported host functions
		case irGet:
			o = append(o, 0x10)           // call
			o = corewasm.AppendULEB(o, 0) // import index 0 = get
		case irGet3:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 1) // import index 1 = get3
		case irAssoc:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 2) // import index 2 = assoc
		case irNth:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 3) // import index 3 = nth
		case irConj:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 4) // import index 4 = conj
		case irCount:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 5) // import index 5 = count
		case irFirst:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 6) // import index 6 = first

		case irJumpIfNot:
			pc += 2
			o = append(o, 0xa7)       // i32.wrap_i64
			o = append(o, 0x04, 0x40) // if void
			depth++

		case irJump:
			pc += 2
			o = append(o, 0x05) // else

		case irReturn:
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth+1)
			if depth > 0 && pc < len(code) && code[pc] != irJump {
				o = append(o, 0x05)
			}

		case irCallSelf:
			pc += 2                                 // skip nargs
			o = append(o, 0x10)                     // call
			o = corewasm.AppendULEB(o, mainFuncIdx) // self = main function index

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			for i := nargs - 1; i >= 0; i-- {
				o = append(o, 0x21)
				o = corewasm.AppendULEB(o, i)
			}
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth)
			pc = len(code) // dead code after recur

		default:
			return nil
		}
	}

	for depth > 0 {
		o = append(o, 0x0b)
		depth--
	}
	o = append(o, 0x0b)       // end loop
	o = append(o, 0x42, 0x00) // i64.const 0
	o = append(o, 0x0b)       // end block
	o = append(o, 0x0b)       // end func
	return o
}

// ---- wasm_helper_backend.go ----
// wasm_multifn.go — experimental one-helper multi-function WASM modules.
//
// This removes the host boundary for hot loops that call a single captured
// helper function. The caller remains the exported exec function; the helper is
// emitted as a second internal WASM function and irCallSlot becomes a direct
// WASM call. This is intentionally not wired into the default eval path yet.

type wasmMultiKey struct {
	caller *IRProgram
	helper *FnArityExpr
}

var wasmMultiFnCache sync.Map    // map[wasmMultiKey]*WasmProgram
var wasmMultiFnProgFail sync.Map // map[*IRProgram]bool for no-helper/auto-rejected callers

func wasmGetCachedWithOneHelper(prog *IRProgram, slots []coretypes.Object) *WasmProgram {
	if !corert.WasmMultiFnEnabled() {
		return nil
	}
	if _, failed := wasmMultiFnProgFail.Load(prog); failed {
		return nil
	}
	helperSlot, helperFn, helperProg, helperParams, ok := findSingleWasmHelper(prog, slots)
	if !ok {
		wasmMultiFnProgFail.Store(prog, true)
		return nil
	}
	key := wasmMultiKey{caller: prog, helper: &helperFn.fnExpr.arities[0]}
	if v, ok := wasmMultiFnCache.Load(key); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFail {
			return nil
		}
		return wp
	}
	wp := wasmCompileWithOneHelper(prog, helperSlot, helperProg, helperParams)
	if wp == nil {
		wasmMultiFnCache.Store(key, wasmFail)
		return nil
	}
	wasmMultiFnCache.Store(key, wp)
	return wp
}

func findSingleWasmHelper(prog *IRProgram, slots []coretypes.Object) (int, *Fn, *IRProgram, int, bool) {
	model := prog.neutralModel()
	if model == nil {
		return 0, nil, nil, 0, false
	}
	code := model.Code
	pc := 0
	helperSlot := -1
	helperNArgs := -1
	helperCalls := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot, irJumpIfNot, irJump, irCallSelf, irBuildVec, irNthStringASCII:
			pc += 2
		case irCallSlot:
			slot := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			helperCalls++
			if helperSlot < 0 {
				helperSlot = slot
				helperNArgs = nargs
			} else if helperSlot != slot || helperNArgs != nargs {
				return 0, nil, nil, 0, false
			}
		case irRecur:
			pc += 4
			tgt := int(code[pc-2])<<8 | int(code[pc-1])
			if tgt != 0 {
				pc += 2
			}
		}
	}
	if helperSlot < 0 || helperSlot >= len(slots) {
		return 0, nil, nil, 0, false
	}
	helperFn, ok := slots[helperSlot].(*Fn)
	if !ok || len(helperFn.fnExpr.arities) != 1 || len(helperFn.fnExpr.arities[0].args) != helperNArgs {
		return 0, nil, nil, 0, false
	}
	helperProg := irCompileFn(helperFn)
	if helperProg == nil || helperProg.hasSelf {
		return 0, nil, nil, 0, false
	}
	helperModel := helperProg.neutralModel()
	if helperModel == nil || !corewasm.Eligible(helperModel.Code) {
		return 0, nil, nil, 0, false
	}
	if !corewasm.EligibleWithHelper(model.Code, helperSlot) {
		return 0, nil, nil, 0, false
	}
	// Multi-function WASM: enable for both integer and float helpers.
	// Originally gated because float helpers were believed to regress,
	// but 5x median probes show no regression vs auto (within noise).
	if !corert.WasmMultiFnForce() && helperCalls == 0 {
		return 0, nil, nil, 0, false
	}
	return helperSlot, helperFn, helperProg, helperNArgs, true
}

func wasmCompileWithOneHelper(prog *IRProgram, helperSlot int, helperProg *IRProgram, helperParams int) *WasmProgram {
	model := prog.neutralModel()
	if helperProg == nil {
		return nil
	}
	helperModel := helperProg.neutralModel()
	if model == nil || helperModel == nil {
		return nil
	}
	useFloat := corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0) || corewasm.UsesFloat(helperModel.Code, len(helperModel.FloatConsts) > 0)
	callerBody := compileWasmBodyWithHelper(prog, useFloat, helperSlot, 1)
	if callerBody == nil {
		return nil
	}
	helperBody := compileWasmBodyWithHelperParams(helperProg, useFloat, -1, -1, helperParams)
	if helperBody == nil {
		return nil
	}
	valType := corewasm.ValTypeI64
	if useFloat {
		valType = corewasm.ValTypeF64
	}
	bin := corewasm.TwoFuncExecModule(model.NumSlots, helperParams, valType, callerBody, helperBody)

	rt := getWasmRT()
	ctx := context.Background()
	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return nil
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(corewasm.NextWasmModuleName()))
	if err != nil {
		return nil
	}
	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		return nil
	}
	return &WasmProgram{mod: mod, execFn: execFn, useFloat: useFloat, hasImports: false, constants: prog.constants}
}

// ---- wasm_host_funcs.go ----
// wasm_host.go — Host function imports for WASM modules.
//
// Provides Joker collection operations (get, assoc, nth, conj, first, count)
// as imported host functions that WASM-compiled loops can call.
//
// Objects are passed as opaque handles (uint64 indices into a per-execution
// object table). Numeric values (Int, Double) are passed directly as i64/f64.
//
// The object table is thread-local to each wasmExec call, stored in a
// context value so host functions can access it.

// wasmHostModuleName is the import module name for Joker host functions.
const wasmHostModuleName = corewasm.HostModuleName

var wasmHostRegistered sync.Once

// registerWasmHost registers the "joker" host module with collection operations.
func registerWasmHost(rt wazero.Runtime) {
	wasmHostRegistered.Do(func() {
		ctx := context.Background()
		builder := rt.NewHostModuleBuilder(wasmHostModuleName)

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64, key uint64) uint64 {
			return corewasm.HostGet(corewasm.GetObjectTable(ctx), collHandle, key, 0)
		}).Export("get")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64, key uint64, def uint64) uint64 {
			return corewasm.HostGet(corewasm.GetObjectTable(ctx), collHandle, key, def)
		}).Export("get3")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64, key uint64, val uint64) uint64 {
			return corewasm.HostAssoc(corewasm.GetObjectTable(ctx), collHandle, key, val)
		}).Export("assoc")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64, idx uint64) uint64 {
			return corewasm.HostNth(corewasm.GetObjectTable(ctx), collHandle, idx, 0, func(coll coretypes.Object, i int) (coretypes.Object, bool) {
				if v, ok := coll.(*corecollections.ArrayVector); ok && i >= 0 && i < len(v.Arr) {
					return v.Arr[i], true
				}
				return nil, false
			})
		}).Export("nth")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64, val uint64) uint64 {
			return corewasm.HostConj(corewasm.GetObjectTable(ctx), collHandle, val)
		}).Export("conj")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64) uint64 {
			return corewasm.HostFirst(corewasm.GetObjectTable(ctx), collHandle, 0, func(coll coretypes.Object) (coretypes.Object, bool) {
				if v, ok := coll.(*corecollections.ArrayVector); ok && len(v.Arr) > 0 {
					return v.Arr[0], true
				}
				return nil, false
			})
		}).Export("first")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64) uint64 {
			return corewasm.HostCount(corewasm.GetObjectTable(ctx), collHandle)
		}).Export("count")

		builder.Instantiate(ctx)
	})
}

// Ensure api import is used
var _ api.Module

// ---- wasm_mem_nth_backend.go ----
// wasm_mem_nth.go — WASM f64 codegen with linear memory for vector nth.
//
// For loops that use f64 arithmetic + vector nth + optional helper calls,
// vector elements are copied into WASM linear memory before execution.
// The nth opcode becomes an f64.load from computed memory address.
// This eliminates all Go↔WASM boundary crossings for nth.

var wasmMemNthCache sync.Map

type wasmMemNthKey struct {
	prog   *IRProgram
	helper *IRProgram
}

// wasmMemNthStaticEligible is a fast static check (no slot inspection).
func wasmMemNthStaticEligible(prog *IRProgram) bool {
	if !corert.WasmMemNthEnabled() {
		return false
	}
	model := prog.neutralModel()
	if model == nil {
		return false
	}
	code := model.Code
	pc := 0
	hasNth := false
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irDiv, irRem, irInc, irDec,
			irLt, irGte, irGt, irLte, irEq, irIsZero, irReturn, irSqrt:
			// ok
		case irNth:
			hasNth = true
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				return false
			}
		default:
			return false
		}
	}
	return hasNth
}

// Requires: f64 arithmetic, irNth on captured vectors, optional irCallSlot.
func wasmMemNthEligible(prog *IRProgram, slots []coretypes.Object) bool {
	if prog == nil {
		return false
	}
	model := prog.neutralModel()
	if model == nil || len(slots) < model.NumSlots {
		return false
	}
	// Check if any slot is a Double (indicates float loop)
	hasFloat := false
	for _, s := range slots {
		if _, ok := s.(coretypes.Double); ok {
			hasFloat = true
			break
		}
	}
	if !hasFloat {
		hasFloat = corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0)
	}
	if !hasFloat {
		return false
	}
	code := model.Code
	pc := 0
	hasNth := false
	nthSlots := make(map[int]bool) // which slots are used as nth collection args
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irDiv, irRem, irInc, irDec,
			irLt, irGte, irGt, irLte, irEq, irIsZero, irReturn, irSqrt:
			// ok
		case irNth:
			hasNth = true
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				return false
			}
		default:
			return false
		}
	}
	if !hasNth {
		return false
	}
	// Find which slots are loaded before nth and verify they're vectors
	pc = 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLoadSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Check if next non-load op is nth
			if pc < len(code) {
				nextOp := code[pc]
				if nextOp == irLoadSlot {
					// Pattern: load coll, load idx, nth
					nextSlot := int(code[pc+1])<<8 | int(code[pc+2])
					if pc+3 < len(code) && code[pc+3] == irNth {
						_ = nextSlot
						nthSlots[slotIdx] = true
					}
				}
			}
		case irLiteral, irStoreSlot:
			pc += 2
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				pc += 2
			}
		default:
			// single byte
		}
	}
	// Verify that nth collection slots hold ArrayVectors
	for slot := range nthSlots {
		if slot >= len(slots) {
			return false
		}
		if _, ok := slots[slot].(*corecollections.ArrayVector); !ok {
			return false
		}
	}
	return true
}

type wasmMemNthCached struct {
	wp         *WasmProgram
	vecSlotIdx []int     // initSlots indices that hold vectors
	memOffsets []int     // byte offset for each vecSlotIdx
	lastVecPtr []uintptr // last-written vector pointer per slot
	paramsBuf  []uint64  // reusable params buffer
	buf8       [8]byte   // reusable byte buffer for f64 writes
}

// wasmMemNthCompileAndExec compiles and executes the loop with linear memory nth.
func wasmMemNthCompileAndExec(prog *IRProgram, slots []coretypes.Object) coretypes.Object {
	if !wasmMemNthEligible(prog, slots) {
		return nil
	}
	helperSlot, helperProg := findHelperForMemNth(prog, slots)

	key := wasmMemNthKey{prog: prog, helper: helperProg}
	var c *wasmMemNthCached
	if v, ok := wasmMemNthCache.Load(key); ok {
		if v == nil {
			return nil // cached failure
		}
		c = v.(*wasmMemNthCached)
	} else {
		wp := buildMemNthModule(prog, helperSlot, helperProg)
		if wp == nil {
			wasmMemNthCache.Store(key, nil)
			return nil
		}
		// Identify vector slots
		vecSlots := findVecSlots(prog, slots)
		var vecIdx []int
		var memOff []int
		offset := 0
		for _, vs := range vecSlots {
			vecIdx = append(vecIdx, vs.slot)
			memOff = append(memOff, offset)
			offset += len(vs.vec.Arr) * 8
		}
		model := prog.neutralModel()
		if model == nil {
			wasmMemNthCache.Store(key, nil)
			return nil
		}
		c = &wasmMemNthCached{
			wp:         wp,
			vecSlotIdx: vecIdx,
			memOffsets: memOff,
			lastVecPtr: make([]uintptr, len(vecIdx)),
			paramsBuf:  make([]uint64, model.NumSlots),
		}
		wasmMemNthCache.Store(key, c)
	}

	// Write vector data to memory — skip if same vector pointer
	mem := c.wp.mod.ExportedMemory("memory")
	if mem == nil {
		return nil
	}
	for vi, slotIdx := range c.vecSlotIdx {
		vec := slots[slotIdx].(*corecollections.ArrayVector)
		vecPtr := reflect.ValueOf(vec).Pointer()
		if vecPtr != c.lastVecPtr[vi] {
			base := c.memOffsets[vi]
			for i, obj := range vec.Arr {
				var fv float64
				switch v := obj.(type) {
				case coretypes.Double:
					fv = v.D
				case coretypes.Int:
					fv = float64(v.I)
				default:
					return nil
				}
				binary.LittleEndian.PutUint64(c.buf8[:], math.Float64bits(fv))
				if !mem.Write(uint32(base+i*8), c.buf8[:]) {
					return nil
				}
			}
			c.lastVecPtr[vi] = vecPtr
		}
	}

	// Build params — reuse buffer
	for i, s := range slots {
		switch v := s.(type) {
		case coretypes.Int:
			c.paramsBuf[i] = math.Float64bits(float64(v.I))
		case coretypes.Double:
			c.paramsBuf[i] = math.Float64bits(v.D)
		default:
			// coretypes.Vector slot: pass memory byte offset
			for vi, si := range c.vecSlotIdx {
				if si == i {
					c.paramsBuf[i] = math.Float64bits(float64(c.memOffsets[vi]))
					break
				}
			}
		}
	}

	ctx := context.Background()
	if err := c.wp.execFn.CallWithStack(ctx, c.paramsBuf); err != nil {
		return nil
	}
	return coretypes.Double{D: math.Float64frombits(c.paramsBuf[0])}
}

type vecSlotInfo struct {
	slot int
	vec  *corecollections.ArrayVector
}

func findVecSlots(prog *IRProgram, slots []coretypes.Object) []vecSlotInfo {
	// Find slots loaded before irNth
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	code := model.Code
	var result []vecSlotInfo
	seen := make(map[int]bool)
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLoadSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if pc+3 < len(code) && code[pc] == irLoadSlot && code[pc+3] == irNth {
				if !seen[slotIdx] {
					if v, ok := slots[slotIdx].(*corecollections.ArrayVector); ok {
						result = append(result, vecSlotInfo{slot: slotIdx, vec: v})
						seen[slotIdx] = true
					}
				}
			}
		case irLiteral, irStoreSlot:
			pc += 2
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				pc += 2
			}
		default:
		}
	}
	return result
}

func findHelperForMemNth(prog *IRProgram, slots []coretypes.Object) (int, *IRProgram) {
	model := prog.neutralModel()
	if model == nil {
		return -1, nil
	}
	code := model.Code
	pc := 0
	helperSlot := -1
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irCallSlot:
			s := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			if helperSlot < 0 {
				helperSlot = s
			} else if helperSlot != s {
				return -1, nil
			}
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				pc += 2
			}
		default:
		}
	}
	if helperSlot < 0 || helperSlot >= len(slots) {
		return -1, nil
	}
	fn, ok := slots[helperSlot].(*Fn)
	if !ok {
		return -1, nil
	}
	hp := irCompileFn(fn)
	hm := hp.neutralModel()
	if hp == nil || hm == nil || !corewasm.Eligible(hm.Code) {
		return -1, nil
	}
	return helperSlot, hp
}

func buildMemNthModule(prog *IRProgram, helperSlot int, helperProg *IRProgram) *WasmProgram {
	rt := getWasmRT()
	if rt == nil {
		return nil
	}
	helperFuncIdx := -1
	helperParams := 0
	if helperProg != nil {
		helperFuncIdx = 1
		helperModel := helperProg.neutralModel()
		if helperModel == nil {
			return nil
		}
		helperParams = helperModel.NumSlots
	}
	model := prog.neutralModel()
	if model == nil {
		return nil
	}

	callerBody := buildMemNthBody(prog, helperSlot, helperFuncIdx, model.NumSlots)
	if callerBody == nil {
		return nil
	}
	var helperBody []byte
	if helperProg != nil {
		helperBody = compileWasmBodyWithHelperParams(helperProg, true, -1, -1, helperParams)
		if helperBody == nil {
			return nil
		}
	}

	bin := corewasm.MemoryExportModule(model.NumSlots, helperParams, callerBody, helperBody)
	ctx := context.Background()
	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return nil
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(corewasm.NextWasmModuleName()))
	if err != nil {
		return nil
	}
	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		return nil
	}
	return &WasmProgram{mod: mod, execFn: execFn, useFloat: true, hasImports: false, constants: prog.constants}
}

func buildMemNthBody(prog *IRProgram, helperSlot, helperFuncIdx, numParams int) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	var o []byte
	extra := model.NumSlots - numParams
	// Local decls: extra f64 locals + 1 i32 temp for nth address computation
	if extra > 0 {
		o = append(o, 0x02) // 2 groups
		o = corewasm.AppendULEB(o, extra)
		o = append(o, 0x7c) // f64
		o = append(o, 0x01) // 1 i32
		o = append(o, 0x7f) // i32
	} else {
		o = append(o, 0x01) // 1 group
		o = append(o, 0x01) // 1 i32
		o = append(o, 0x7f)
	}
	i32Temp := model.NumSlots // local index of i32 temp
	o = append(o, 0x02, 0x7c) // block $exit → f64
	o = append(o, 0x03, 0x40) // loop $loop → void

	code := model.Code
	pc := 0
	depth := 0

	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			c := prog.constants[idx]
			var fv float64
			switch v := c.(type) {
			case coretypes.Int:
				fv = float64(v.I)
			case coretypes.Double:
				fv = v.D
			default:
				return nil
			}
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, fv)
		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x20)
			o = corewasm.AppendULEB(o, idx)
		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x21)
			o = corewasm.AppendULEB(o, idx)
		case irAdd:
			o = append(o, 0xa0)
		case irSub:
			o = append(o, 0xa1)
		case irMul:
			o = append(o, 0xa2)
		case irDiv:
			o = append(o, 0xa3)
		case irSqrt:
			o = append(o, 0x9f)
		case irInc:
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, 1.0)
			o = append(o, 0xa0)
		case irDec:
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, 1.0)
			o = append(o, 0xa1)
		case irLt:
			o = append(o, 0x63) // f64.lt → i32
			o = append(o, 0xb7) // f64.convert_i32_s → f64
		case irGte:
			o = append(o, 0x65) // f64.ge → i32
			o = append(o, 0xb7)
		case irGt:
			o = append(o, 0x64) // f64.gt → i32
			o = append(o, 0xb7)
		case irLte:
			o = append(o, 0x66) // f64.le → i32
			o = append(o, 0xb7)
		case irEq:
			o = append(o, 0x61) // f64.eq → i32
			o = append(o, 0xb7)
		case irIsZero:
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, 0.0)
			o = append(o, 0x61)
			o = append(o, 0xb7)

		case irNth:
			// Stack: [base_offset_f64, idx_f64]
			// Compute address: i32(base) + i32(idx) * 8
			o = append(o, 0xaa) // i32.trunc_f64_s (idx → i32)
			o = append(o, 0x21) // local.set i32_temp
			o = corewasm.AppendULEB(o, i32Temp)
			o = append(o, 0xaa) // i32.trunc_f64_s (base → i32)
			o = append(o, 0x20) // local.get i32_temp
			o = corewasm.AppendULEB(o, i32Temp)
			o = append(o, 0x41, 0x08)       // i32.const 8
			o = append(o, 0x6c)             // i32.mul
			o = append(o, 0x6a)             // i32.add
			o = append(o, 0x2b, 0x03, 0x00) // f64.load align=3 offset=0

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			_ = nargs
			if slotIdx != helperSlot || helperFuncIdx < 0 {
				return nil
			}
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, helperFuncIdx)
		case irJumpIfNot:
			pc += 2
			// Comparison results are f64 (0.0 or 1.0), convert to i32 for if
			o = append(o, 0xaa) // i32.trunc_f64_s
			o = append(o, 0x04, 0x40)
			depth++
		case irJump:
			pc += 2
			o = append(o, 0x05)
		case irReturn:
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth+1)
			if depth > 0 && pc < len(code) && code[pc] != irJump {
				o = append(o, 0x05)
			}
		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			for i := nargs - 1; i >= 0; i-- {
				o = append(o, 0x21)
				o = corewasm.AppendULEB(o, i)
			}
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth)
			pc = len(code)
		default:
			return nil
		}
	}
	for depth > 0 {
		o = append(o, 0x0b)
		depth--
	}
	o = append(o, 0x0b)
	o = append(o, 0x44)
	o = corewasm.AppendF64(o, 0.0)
	o = append(o, 0x0b)
	o = append(o, 0x0b)
	return o
}
