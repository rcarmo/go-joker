package core

import (
	"context"
	"encoding/binary"
	"fmt"
	coreirx "github.com/rcarmo/go-joker/core/ir"
	corert "github.com/rcarmo/go-joker/core/runtime"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	"math"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"unicode/utf8"
	"unsafe"

	coreir "github.com/rcarmo/go-joker/core/ir"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corestr "github.com/rcarmo/go-joker/core/types/string"
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

// ---- boxed_exec.go ----

// ---- boxed_exec.go ----
// ---------- Interpreter ----------

func irExec(prog *IRProgram, initSlots []coretypes.Object) coretypes.Object {
	defer traceIRProgramCall(prog, len(initSlots))()
	irProfileExecStart()
	defer irProfileMaybeWrite()
	var slots []coretypes.Object
	if runtimeExec.ProgramNumSlots(prog) <= 16 {
		var buf [16]coretypes.Object
		slots = buf[:runtimeExec.ProgramNumSlots(prog)]
	} else {
		slots = make([]coretypes.Object, runtimeExec.ProgramNumSlots(prog))
	}
	copy(slots, initSlots)
	// Pre-fill captured closure values into their assigned slots
	if !runtimeExec.ApplyProgramCaptureSlots(prog, slots) {
		return nil
	}

	// Escape analysis: convert safe local values to transient builders.
	// Only run if there are actually mutable candidate slots.
	if runtimeExec.HasMutableSlotCandidate(slots) {
		escapeInfo := runtimeExec.ProgramEscapeInfo(prog)
		if escapeInfo == nil {
			return nil
		}
		for i, s := range slots {
			slots[i] = runtimeExec.MutableSlotObject(s, escapeInfo, i)
		}
	}

	var stack []coretypes.Object
	var stackBuf [16]coretypes.Object
	stack = stackBuf[:0]
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	// Frame stack for irCallSelf — avoids recursive irExec calls
	var frameStack *coreirx.FrameStack[coretypes.Object]
	defer func() { coreirx.ReleaseFrameStack(frameStack) }()
	var selfTraceStack []func()

	var irProfPrev byte
	var irProfHasPrev bool
	irProfStarted := irProfileStart()
	defer func() { irProfileFinish(irProfPrev, irProfHasPrev, irProfStarted) }()
loop:
	for pc < len(code) {
		op := code[pc]
		irProfStarted = irProfileOp(irProfPrev, op, irProfHasPrev, irProfStarted)
		irProfPrev, irProfHasPrev = op, true
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			constant, ok := runtimeExec.ProgramConstant(prog, idx)
			if !ok {
				return nil
			}
			stack = append(stack, constant)

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
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Int{I: av.I + bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: float64(av.I) + bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Double{D: av.D + float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: av.D + bv.D})
					continue
				}
			}
			return nil

		case irSub:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Int{I: av.I - bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: float64(av.I) - bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Double{D: av.D - float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: av.D - bv.D})
					continue
				}
			}
			return nil

		case irMul:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Int{I: av.I * bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: float64(av.I) * bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Double{D: av.D * float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: av.D * bv.D})
					continue
				}
			}
			return nil

		case irRem:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if ai, aok := a.(coretypes.Int); aok {
				if bi, bok := b.(coretypes.Int); bok {
					if bi.I == 0 {
						return nil
					}
					stack = append(stack, coretypes.Int{I: ai.I % bi.I})
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
			case coretypes.Int:
				av = float64(x.I)
			case coretypes.Double:
				av = x.D
			default:
				return nil
			}
			switch x := b.(type) {
			case coretypes.Int:
				bv = float64(x.I)
			case coretypes.Double:
				bv = x.D
			default:
				return nil
			}
			if bv == 0 {
				return nil
			}
			stack = append(stack, coretypes.Double{D: av / bv})

		case irInc:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Int:
				stack = append(stack, coretypes.Int{I: av.I + 1})
			case coretypes.Double:
				stack = append(stack, coretypes.Double{D: av.D + 1})
			default:
				return nil
			}

		case irDec:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Int:
				stack = append(stack, coretypes.Int{I: av.I - 1})
			case coretypes.Double:
				stack = append(stack, coretypes.Double{D: av.D - 1})
			default:
				return nil
			}

		case irLt:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I < bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) < bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D < float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D < bv.D})
					continue
				}
			}
			return nil

		case irGte:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I >= bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) >= bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D >= float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D >= bv.D})
					continue
				}
			}
			return nil

		case irGt:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I > bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) > bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D > float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D > bv.D})
					continue
				}
			}
			return nil

		case irLte:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I <= bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) <= bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D <= float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D <= bv.D})
					continue
				}
			}
			return nil

		case irCursorChar:
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.CursorChar(cur)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irCursorNext:
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.CursorNext(cur)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irCursorDone:
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.CursorDone(cur)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irApply:
			argsSeq := stack[len(stack)-1]
			fnObj := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			args, ok := runtimeExec.CallArgs(argsSeq)
			if !ok {
				return nil
			}
			result, ok := runtimeExec.CallObject(fnObj, args)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irThrow:
			v := stack[len(stack)-1]
			runtimeExec.Throw(v)

		case irTryCatch:
			pc += 4
			return nil

		case irPop:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

		case irMakeFn:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnExpr, ok := runtimeExec.ProgramFnExpr(prog, idx)
			if !ok {
				return nil
			}
			fn := runtimeExec.MakeFn(fnExpr, slots)
			stack = append(stack, fn)

		case irBitAnd:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I & b.I})
		case irBitOr:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I | b.I})
		case irBitNot:
			a := stack[len(stack)-1].(coretypes.Int)
			stack = stack[:len(stack)-1]
			stack = append(stack, coretypes.Int{I: ^a.I})
		case irBitShiftLeft:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I << uint(b.I)})
		case irBitShiftRight:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I >> uint(b.I)})

		case irCase:
			// Jump table: dispatch by integer value
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nCases := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			var testVal int
			switch v := slots[slotIdx].(type) {
			case coretypes.Int:
				testVal = v.I
			default:
				// Skip table, jump to default
				pc += nCases * 4
				pc = int(code[pc])<<8 | int(code[pc+1])
				continue
			}
			matched := false
			for i := 0; i < nCases; i++ {
				caseVal := int(int16(code[pc])<<8 | int16(code[pc+1]))
				target := int(code[pc+2])<<8 | int(code[pc+3])
				pc += 4
				if testVal == caseVal {
					pc = target
					matched = true
					break
				}
			}
			if !matched {
				pc = int(code[pc])<<8 | int(code[pc+1])
			}

		case irEq:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I == bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) == bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D == float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D == bv.D})
					continue
				}
			case coretypes.Char:
				if bv, ok := b.(coretypes.Char); ok {
					stack = append(stack, coretypes.Boolean{B: av.Ch == bv.Ch})
					continue
				}
			case coretypes.String:
				if bv, ok := b.(coretypes.String); ok {
					stack = append(stack, coretypes.Boolean{B: av.S == bv.S})
					continue
				}
			}
			stack = append(stack, coretypes.Boolean{B: runtimeExec.Equal(a, b)})

		case irIsZero:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Int:
				stack = append(stack, coretypes.Boolean{B: av.I == 0})
			case coretypes.Double:
				stack = append(stack, coretypes.Boolean{B: av.D == 0})
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
			case coretypes.Boolean:
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
				if frameStack != nil && frameStack.Depth() > 0 {
					if len(selfTraceStack) > 0 {
						exit := selfTraceStack[len(selfTraceStack)-1]
						selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
						exit()
					}
					var sl int
					pc, sl = frameStack.Pop(slots)
					stack = stack[:sl]
					stack = append(stack, NIL)
					continue
				}
				return NIL
			}
			result := stack[len(stack)-1]
			if frameStack != nil && frameStack.Depth() > 0 {
				result = runtimeExec.PersistentResult(result)
				if len(selfTraceStack) > 0 {
					exit := selfTraceStack[len(selfTraceStack)-1]
					selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
					exit()
				}
				var sl int
				pc, sl = frameStack.Pop(slots)
				stack = stack[:sl]
				stack = append(stack, result)
				continue
			}
			return runtimeExec.PersistentResult(result)
		case irGet:
			key := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if _, ok := coll.(coretypes.Gettable); !ok {
				return nil
			}
			stack = append(stack, runtimeExec.Get(coll, key, NIL))

		case irGet3:
			def := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			stack = append(stack, runtimeExec.Get(coll, key, def))

		case irAssoc:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			result, ok := runtimeExec.Assoc(coll, key, val)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irNth:
			idxObj := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			idx, iok := idxObj.(coretypes.Int)
			if !iok {
				return nil
			}
			result, ok := runtimeExec.Nth(coll, idx.I)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irConj:
			val := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			result, ok := runtimeExec.Conj(coll, val)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irSqrt:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Double:
				stack = append(stack, coretypes.Double{D: math.Sqrt(av.D)})
			case coretypes.Int:
				stack = append(stack, coretypes.Double{D: math.Sqrt(float64(av.I))})
			default:
				return nil
			}

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnObj := slots[slotIdx]
			// Fast path: native f64 closure (fn-level or loop-level)
			if fnProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if nativeHelper, ok := runtimeExec.NativeHelper(fnProg); ok {
					var f64buf [4]float64
					var f64args []float64
					if nargs <= len(f64buf) {
						f64args = f64buf[:nargs]
					} else {
						f64args = make([]float64, nargs)
					}
					for i := nargs - 1; i >= 0; i-- {
						switch v := stack[len(stack)-1].(type) {
						case coretypes.Double:
							f64args[i] = v.D
						case coretypes.Int:
							f64args[i] = float64(v.I)
						default:
							f64args[i] = 0
						}
						stack = stack[:len(stack)-1]
					}
					stack = append(stack, coretypes.Double{D: nativeHelper(coreirx.Float64(f64args))})
					continue
				}
			}
			// Slow path
			var args []coretypes.Object
			var argsBuf [4]coretypes.Object
			if nargs > 0 {
				if nargs <= len(argsBuf) {
					args = argsBuf[:nargs]
				} else {
					args = make([]coretypes.Object, nargs)
				}
				for i := nargs - 1; i >= 0; i-- {
					args[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			}
			// Try WASM fn dispatch first, then IR, then tree-walker
			if result, ok := runtimeExec.FnWasmExec(fnObj, args); ok {
				stack = append(stack, result)
				continue
			}
			if baseProg, ok := runtimeExec.FnProgram(fnObj); ok {
				// Try IR — typed executor first, skip if previously failed
				if fnProg := runtimeExec.DispatchArityProgram(baseProg, nargs); runtimeExec.CanExecuteIR(fnProg) {
					callArgs, ok := runtimeExec.FnCallSlots(fnObj, fnProg, args)
					if !ok {
						return nil
					}
					if runtimeExec.CanExecuteTypedIR(fnProg) {
						if result := irExecTyped(fnProg, callArgs); result != nil {
							stack = append(stack, result)
							continue
						}
						runtimeExec.MarkTypedExecutionFailed(fnProg)
					}
					if result := irExec(fnProg, callArgs); result != nil {
						stack = append(stack, result)
						continue
					}
				}
			}
			// Fallback to normal Fn.Call
			result, ok := runtimeExec.CallObjectWithSyntheticCallExpr(fnObj, args)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Use frame stack for bounded recursion, fall back to
			// recursive irExec for deep/exponential recursion.
			if frameStack == nil {
				frameStack = coreirx.NewFrameStack[coretypes.Object](runtimeExec.ProgramNumSlots(prog))
			}
			if frameStack.Depth() < 512 {
				selfTraceStack = append(selfTraceStack, traceIRProgramCall(prog, nargs))
				frameStack.Push(pc, slots, len(stack)-nargs)
				for i := nargs - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
				// Only clear slots beyond nargs if there are captures
				if runtimeExec.ProgramHasCaptureSlots(prog) {
					for i := nargs; i < len(slots); i++ {
						slots[i] = nil
					}
					if !runtimeExec.ApplyProgramCaptureSlots(prog, slots) {
						return nil
					}
				}
				pc = 0
			} else {
				// Deep recursion: fall back to recursive call
				var args []coretypes.Object
				var argsBuf [4]coretypes.Object
				if nargs <= len(argsBuf) {
					args = argsBuf[:nargs]
				} else {
					args = make([]coretypes.Object, nargs)
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
			}

		case irFirst:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.First(a)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			arr := make([]coretypes.Object, n)
			for i := n - 1; i >= 0; i-- {
				arr[i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, runtimeExec.BuildVector(arr))

		case irStr1:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			stack = append(stack, runtimeExec.Str1(a))

		case irNthStringASCII:
			idxConst := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			idxObj := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			idx, ok := idxObj.(coretypes.Int)
			if !ok {
				return nil
			}
			result, ok := runtimeExec.NthASCIIStringConst(prog, idxConst, idx.I)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irStr2:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, runtimeExec.Str2(a, b))

		case irCount:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			count, ok := runtimeExec.Count(a)
			if !ok {
				return nil
			}
			stack = append(stack, coretypes.Int{I: count})

		case irToTransient:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.ToTransient(a)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irAssocBang:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			result, ok := runtimeExec.AssocBang(coll, key, val)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irToPersistent:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.ToPersistent(a)
			if !ok {
				return nil
			}
			stack = append(stack, result)
		case irIntCast:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch v := a.(type) {
			case coretypes.Char:
				stack = append(stack, coretypes.Int{I: int(v.Ch)})
			case coretypes.Int:
				stack = append(stack, v)
			case coretypes.Double:
				stack = append(stack, coretypes.Int{I: int(v.D)})
			default:
				return nil
			}

		case irSubs:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if nargs == 3 {
				end := stack[len(stack)-1]
				start := stack[len(stack)-2]
				sObj := stack[len(stack)-3]
				stack = stack[:len(stack)-3]
				s := sObj.(coretypes.String).S
				si := start.(coretypes.Int).I
				ei := end.(coretypes.Int).I
				if coretypes.StringIsASCII(s) {
					stack = append(stack, coretypes.String{S: s[si:ei]})
				} else {
					runes := []rune(s)
					stack = append(stack, coretypes.String{S: string(runes[si:ei])})
				}
			} else {
				start := stack[len(stack)-1]
				sObj := stack[len(stack)-2]
				stack = stack[:len(stack)-2]
				s := sObj.(coretypes.String).S
				si := start.(coretypes.Int).I
				if coretypes.StringIsASCII(s) {
					stack = append(stack, coretypes.String{S: s[si:]})
				} else {
					runes := []rune(s)
					stack = append(stack, coretypes.String{S: string(runes[si:])})
				}
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

// ---- typed_exec.go ----
func irExecTyped(prog *IRProgram, initSlots []coretypes.Object) coretypes.Object {
	defer traceIRProgramCall(prog, len(initSlots))()
	irProfileExecStart()
	defer irProfileMaybeWrite()
	analysis := runtimeExec.ProgramAnalysis(prog)
	if !irTypedEligible(analysis) {
		return nil
	}
	var slotBuf [16]irValue
	var slots []irValue
	numSlots := runtimeExec.ProgramNumSlots(prog)
	if numSlots <= len(slotBuf) {
		slots = slotBuf[:numSlots]
	} else {
		slots = make([]irValue, numSlots)
	}
	for i := 0; i < len(initSlots) && i < len(slots); i++ {
		v := objectToIRValue(initSlots[i])
		if v.tag == irValString && i < len(analysis.StringAppendSlots) && (analysis.StringAppendSlots[i] || analysis.StringPrependSlots[i]) {
			buf := make([]byte, len(v.str()), len(v.str())+16)
			copy(buf, v.str())
			v = irMakeStringBuilder(buf, v.i, v.boolean())
		}
		slots[i] = v
	}
	// Pre-fill captured closure values into their assigned slots
	if !runtimeExec.ApplyProgramTypedCaptureSlots(prog, slots) {
		return nil
	}

	var stackBuf [32]irValue
	stack := stackBuf[:0]
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	// Frame stack for irCallSelf — avoids recursive irExecTyped calls
	var typedFrameStack *coreirx.FrameStack[irValue]
	defer func() { coreirx.ReleaseFrameStack(typedFrameStack) }()
	var selfTraceStack []func()
	var irProfPrev byte
	var irProfHasPrev bool
	irProfStarted := irProfileStart()
	defer func() { irProfileFinish(irProfPrev, irProfHasPrev, irProfStarted) }()

	for pc < len(code) {
		op := code[pc]
		irProfStarted = irProfileOp(irProfPrev, op, irProfHasPrev, irProfStarted)
		irProfPrev, irProfHasPrev = op, true
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			constant, ok := runtimeExec.ProgramConstant(prog, idx)
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(constant))
		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if idx < 0 || idx >= len(slots) {
				return nil
			}
			stack = append(stack, slots[idx])
		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if idx < 0 || idx >= len(slots) || len(stack) == 0 {
				return nil
			}
			slots[idx] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case irAdd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i + b.i})
			} else {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					return nil
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					return nil
				}
				stack = append(stack, irValue{tag: irValDouble, f: af + bf})
			}
		case irSub:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i - b.i})
			} else if a.tag == irValDouble || b.tag == irValDouble {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					return nil
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					return nil
				}
				stack = append(stack, irValue{tag: irValDouble, f: af - bf})
			} else {
				return nil
			}
		case irMul:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i * b.i})
			} else if a.tag == irValDouble || b.tag == irValDouble {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					return nil
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					return nil
				}
				stack = append(stack, irValue{tag: irValDouble, f: af * bf})
			} else {
				return nil
			}
		case irDiv:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			af, bf := 0.0, 0.0
			if a.tag == irValDouble {
				af = a.f
			} else if a.tag == irValInt {
				af = float64(a.i)
			} else {
				return nil
			}
			if b.tag == irValDouble {
				bf = b.f
			} else if b.tag == irValInt {
				bf = float64(b.i)
			} else {
				return nil
			}
			if bf == 0 {
				return nil
			}
			stack = append(stack, irValue{tag: irValDouble, f: af / bf})
		case irRem:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag != irValInt || b.tag != irValInt || b.i == 0 {
				return nil
			}
			stack = append(stack, irValue{tag: irValInt, i: a.i % b.i})
		case irInc:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValInt {
				return nil
			}
			stack = append(stack, irValue{tag: irValInt, i: a.i + 1})
		case irDec:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValInt {
				return nil
			}
			stack = append(stack, irValue{tag: irValInt, i: a.i - 1})
		case irLt:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i < b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f < b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f < float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) < b.f))
			} else {
				return nil
			}
		case irGte:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i >= b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f >= b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f >= float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) >= b.f))
			} else {
				return nil
			}
		case irGt:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i > b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f > b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f > float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) > b.f))
			} else {
				return nil
			}
		case irLte:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i <= b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f <= b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f <= float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) <= b.f))
			} else {
				return nil
			}

		case irCursorChar:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			result, ok := runtimeExec.CursorChar(v.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irCursorNext:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			result, ok := runtimeExec.CursorNext(v.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irCursorDone:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			result, ok := runtimeExec.CursorDone(v.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irApply:
			argsVal := stack[len(stack)-1]
			fnVal := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			fnObj := fnVal.object()
			argsObj := argsVal.object()
			args, ok := runtimeExec.CallArgs(argsObj)
			if !ok {
				return nil
			}
			result, ok := runtimeExec.CallObject(fnObj, args)
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irThrow:
			v := stack[len(stack)-1]
			runtimeExec.Throw(v.object())

		case irTryCatch:
			pc += 4
			return nil

		case irPop:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

		case irMakeFn:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnExpr, ok := runtimeExec.ProgramFnExpr(prog, idx)
			if !ok {
				return nil
			}
			capturedSlots := make([]coretypes.Object, len(slots))
			for i, v := range slots {
				capturedSlots[i] = v.object()
			}
			fn := runtimeExec.MakeFn(fnExpr, capturedSlots)
			stack = append(stack, objectToIRValue(fn))

		case irBitAnd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i & b.i})
			} else {
				return nil
			}
		case irBitOr:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i | b.i})
			} else {
				return nil
			}
		case irBitNot:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: ^a.i})
			} else {
				return nil
			}
		case irBitShiftLeft:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i << uint(b.i)})
			} else {
				return nil
			}
		case irBitShiftRight:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i >> uint(b.i)})
			} else {
				return nil
			}

		case irCase:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nCases := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			v := slots[slotIdx]
			if v.tag != irValInt {
				pc += nCases * 4
				pc = int(code[pc])<<8 | int(code[pc+1])
				continue
			}
			testVal := v.i
			matched := false
			for i := 0; i < nCases; i++ {
				caseVal := int(int16(code[pc])<<8 | int16(code[pc+1]))
				target := int(code[pc+2])<<8 | int(code[pc+3])
				pc += 4
				if testVal == caseVal {
					pc = target
					matched = true
					break
				}
			}
			if !matched {
				pc = int(code[pc])<<8 | int(code[pc+1])
			}

		case irEq:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			v, ok := irValueEq(a, b)
			if !ok {
				return nil
			}
			stack = append(stack, v)
		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			cond := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !cond.truthy() {
				pc = target
			}
		case irJump:
			pc = int(code[pc])<<8 | int(code[pc+1])
		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			baseSlot := 0
			if target != 0 {
				baseSlot = int(code[pc])<<8 | int(code[pc+1])
				pc += 2
			}
			for i := nargs - 1; i >= 0; i-- {
				slots[baseSlot+i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			pc = target
			stack = stack[:0]
		case irReturn:
			if len(stack) == 0 {
				if typedFrameStack != nil && typedFrameStack.Depth() > 0 {
					if len(selfTraceStack) > 0 {
						exit := selfTraceStack[len(selfTraceStack)-1]
						selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
						exit()
					}
					var sl int
					pc, sl = typedFrameStack.Pop(slots)
					stack = stack[:sl]
					stack = append(stack, irValue{tag: irValNil})
					continue
				}
				return NIL
			}
			result := stack[len(stack)-1]
			if typedFrameStack != nil && typedFrameStack.Depth() > 0 {
				if len(selfTraceStack) > 0 {
					exit := selfTraceStack[len(selfTraceStack)-1]
					selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
					exit()
				}
				var sl int
				pc, sl = typedFrameStack.Pop(slots)
				stack = stack[:sl]
				stack = append(stack, result)
				continue
			}
			return result.object()
		case irGet:
			key := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if coll.tag != irValStringIntMap {
				return nil
			}
			k, ok := irValueStringKey(key)
			if !ok {
				return nil
			}
			if v, ok := coll.stringIntMap()[k]; ok {
				stack = append(stack, irValue{tag: irValInt, i: v})
			} else {
				stack = append(stack, irValue{tag: irValNil})
			}
		case irGet3:
			def := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if coll.tag != irValStringIntMap || def.tag != irValInt {
				return nil
			}
			k, ok := irValueStringKey(key)
			if !ok {
				return nil
			}
			if v, ok := coll.stringIntMap()[k]; ok {
				stack = append(stack, irValue{tag: irValInt, i: v})
			} else {
				stack = append(stack, def)
			}
		case irAssoc:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if coll.tag == irValStringIntMap && val.tag == irValInt {
				k, ok := irValueStringKey(key)
				if !ok {
					return nil
				}
				if coll.stringIntMap() == nil {
					coll.setStringIntMap(make(map[string]int))
				}
				coll.stringIntMap()[k] = val.i
				stack = append(stack, coll)
			} else if coll.tag == irValIntVector && key.tag == irValInt && val.tag == irValInt {
				if key.i < 0 || key.i > len(coll.intVec()) {
					return nil
				}
				iv := coll.intVec()
				if key.i == len(iv) {
					iv = append(iv, val.i)
				} else {
					iv[key.i] = val.i
				}
				coll.setIntVec(iv)
				stack = append(stack, coll)
			} else {
				// General assoc path for coretypes.Object types (vector of doubles, etc.)
				result, ok := runtimeExec.Assoc(coll.object(), key.object(), val.object())
				if !ok {
					return nil
				}
				stack = append(stack, objectToIRValue(result))
			}
		case irNth:
			idx := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if idx.tag != irValInt || idx.i < 0 {
				return nil
			}
			if coll.tag == irValString {
				if coll.boolean() {
					if idx.i >= len(coll.str()) {
						return nil
					}
					stack = append(stack, irMakeChar(rune(coll.str()[idx.i])))
				} else {
					n := 0
					found := false
					for _, r := range coll.str() {
						if n == idx.i {
							stack = append(stack, irMakeChar(r))
							found = true
							break
						}
						n++
					}
					if !found {
						return nil
					}
				}
			} else if coll.tag == irValIntVector {
				if idx.i >= len(coll.intVec()) {
					return nil
				}
				stack = append(stack, irValue{tag: irValInt, i: coll.intVec()[idx.i]})
			} else if coll.tag == irValObject {
				obj, ok := runtimeExec.Nth(coll.obj(), idx.i)
				if !ok {
					return nil
				}
				stack = append(stack, objectToIRValue(obj))
			} else {
				return nil
			}
		case irNthStringASCII:
			idxConst := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			idx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if idx.tag != irValInt {
				return nil
			}
			constant, ok := runtimeExec.ProgramConstant(prog, idxConst)
			if !ok {
				return nil
			}
			s := constant.(coretypes.String).S
			if idx.i < 0 || idx.i >= len(s) {
				return nil
			}
			stack = append(stack, irMakeChar(rune(s[idx.i])))
		case irStr1:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValString || a.tag == irValStringBuilder {
				stack = append(stack, a)
			} else {
				stack = append(stack, stringToIRValue(irValueToString(a)))
			}
		case irStr2:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValStringBuilder {
				bs := irValueToString(b)
				abuf := append(a.bytes(), bs...)
				ascii := a.isASCII()
				if ascii {
					for i := 0; i < len(bs); i++ {
						if bs[i] >= utf8.RuneSelf {
							ascii = false
							break
						}
					}
				}
				rc := a.i
				if ascii {
					rc += len(bs)
				} else {
					rc = irStringRuneCount(string(abuf))
				}
				stack = append(stack, irMakeStringBuilder(abuf, rc, ascii))
			} else if b.tag == irValStringBuilder {
				prefix := irValueToString(a)
				if prefix != "" {
					bbuf := b.bytes()
					newBuf := make([]byte, len(prefix)+len(bbuf))
					copy(newBuf, prefix)
					copy(newBuf[len(prefix):], bbuf)
					ascii := b.isASCII()
					if ascii {
						for i := 0; i < len(prefix); i++ {
							if prefix[i] >= utf8.RuneSelf {
								ascii = false
								break
							}
						}
					}
					rc := b.i
					if ascii {
						rc += len(prefix)
					} else {
						rc = irStringRuneCount(string(newBuf))
					}
					b = irMakeStringBuilder(newBuf, rc, ascii)
				}
				stack = append(stack, b)
			} else {
				stack = append(stack, stringToIRValue(irValueToString(a)+irValueToString(b)))
			}
		case irCount:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValString || a.tag == irValStringBuilder {
				stack = append(stack, irValue{tag: irValInt, i: a.i})
			} else if a.tag == irValStringIntMap {
				stack = append(stack, irValue{tag: irValInt, i: len(a.stringIntMap())})
			} else if a.tag == irValIntVector {
				stack = append(stack, irValue{tag: irValInt, i: len(a.intVec())})
			} else if a.tag == irValObject {
				count, ok := runtimeExec.Count(a.obj())
				if !ok {
					return nil
				}
				stack = append(stack, irValue{tag: irValInt, i: count})
			} else {
				return nil
			}
		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Load fn from typed slots (supports captures beyond initSlots)
			var fnObj coretypes.Object
			if slotIdx < len(initSlots) {
				fnObj = initSlots[slotIdx]
			} else {
				fnObj = slots[slotIdx].object()
			}
			// Fast path: native f64 closure (zero boxing)
			if fnProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if nativeHelper, ok := runtimeExec.NativeHelper(fnProg); ok {
					// Call native helper with stack-allocated args for common arities.
					var f64buf [4]float64
					var f64args []float64
					if nargs <= len(f64buf) {
						f64args = f64buf[:nargs]
					} else {
						f64args = make([]float64, nargs)
					}
					for i := nargs - 1; i >= 0; i-- {
						v := stack[len(stack)-1]
						stack = stack[:len(stack)-1]
						if v.tag == irValDouble {
							f64args[i] = v.f
						} else if v.tag == irValInt {
							f64args[i] = float64(v.i)
						}
					}
					r := nativeHelper(coreirx.Float64(f64args))
					stack = append(stack, irValue{tag: irValDouble, f: r})
					continue
				}
			}
			// Pop args as irValues (no boxing)
			var typedArgBuf [4]irValue
			var typedArgs []irValue
			if nargs <= 4 {
				typedArgs = typedArgBuf[:nargs]
			} else {
				typedArgs = make([]irValue, nargs)
			}
			for i := nargs - 1; i >= 0; i-- {
				typedArgs[i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			var result coretypes.Object
			if baseProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if baseProg != nil && runtimeExec.HasNativeHelper(baseProg) {
					// Already handled above
				} else if fnProg := runtimeExec.DispatchArityProgram(baseProg, nargs); runtimeExec.CanExecuteIR(fnProg) {
					routedToIR := false
					if runtimeExec.CanExecuteTypedIR(fnProg) {
						// FAST PATH: typed sub-call without coretypes.Object boxing
						// Only for pure numeric programs (no collections/strings)
						subAnalysis := runtimeExec.ProgramAnalysis(fnProg)
						if irTypedEligible(subAnalysis) && !subAnalysis.UsesCollection && !subAnalysis.UsesString && !subAnalysis.HasCallSlot {
							var subBuf [16]irValue
							var subSlots []irValue
							numSlots := runtimeExec.ProgramNumSlots(fnProg)
							if numSlots < nargs {
								return nil
							}
							if numSlots <= 16 {
								subSlots = subBuf[:numSlots]
							} else {
								subSlots = make([]irValue, numSlots)
							}
							copy(subSlots[:nargs], typedArgs)
							// Resolve captures
							if !runtimeExec.InstallFnTypedEnvCaptures(fnObj, fnProg, subSlots) {
								return nil
							}
							// Execute inline with typed slots
							subResult := irExecTypedInline(fnProg, subSlots)
							if subResult.tag != 0 || subResult.i != 0 || subResult.f != 0 {
								stack = append(stack, subResult)
								continue
							}
							runtimeExec.MarkTypedExecutionFailed(fnProg)
						}
					}
					// Fallback: box args
					var argsBuf [4]coretypes.Object
					args := runtimeExec.ObjectsFromTypedValues(typedArgs, argsBuf[:])
					callArgs, ok := runtimeExec.FnCallSlots(fnObj, fnProg, args)
					if !ok {
						return nil
					}
					if r := irExec(fnProg, callArgs); r != nil {
						result = r
						routedToIR = true
					} else {
						runtimeExec.MarkBoxedExecutionFailed(fnProg)
					}
					if !routedToIR && result == nil {
						return nil
					}
				}
				if result == nil {
					var args2 [4]coretypes.Object
					a := runtimeExec.ObjectsFromTypedValues(typedArgs, args2[:])
					var ok bool
					result, ok = runtimeExec.CallObject(fnObj, a)
					if !ok {
						return nil
					}
				}
			} else {
				var args3 [4]coretypes.Object
				a := runtimeExec.ObjectsFromTypedValues(typedArgs, args3[:])
				var ok bool
				result, ok = runtimeExec.CallObject(fnObj, a)
				if !ok {
					return nil
				}
			}
			stack = append(stack, objectToIRValue(result))

		case irSqrt:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: math.Sqrt(a.f)})
			} else if a.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: math.Sqrt(float64(a.i))})
			} else {
				return nil
			}

		case irConj:
			val := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if coll.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.Conj(coll.obj(), val.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if typedFrameStack == nil {
				typedFrameStack = coreirx.NewFrameStack[irValue](runtimeExec.ProgramNumSlots(prog))
			}
			if typedFrameStack.Depth() < 256 {
				// Save current state and restart
				selfTraceStack = append(selfTraceStack, traceIRProgramCall(prog, nargs))
				typedFrameStack.Push(pc, slots, len(stack)-nargs)
				// Pop args directly into slots (no intermediate copy)
				for i := nargs - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
				// Clear only non-capture working slots
				if !runtimeExec.ClearTypedNonCaptureSlots(prog, slots, nargs) {
					return nil
				}
				pc = 0
			} else {
				// Deep recursion: box args and fall back
				args := make([]coretypes.Object, nargs)
				for i := nargs - 1; i >= 0; i-- {
					args[i] = stack[len(stack)-1].object()
					stack = stack[:len(stack)-1]
				}
				result := irExecTyped(prog, args)
				if result == nil {
					result = irExec(prog, args)
				}
				if result == nil {
					return nil
				}
				stack = append(stack, objectToIRValue(result))
			}

		case irFirst:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.First(a.obj())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			arr := make([]coretypes.Object, n)
			for i := n - 1; i >= 0; i-- {
				arr[i] = stack[len(stack)-1].object()
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, irMakeObject(runtimeExec.BuildVector(arr)))

		case irToTransient:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.ToTransient(a.obj())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irAssocBang:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			tv := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if tv.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.AssocBang(tv.obj(), key.object(), val.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irToPersistent:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.ToPersistent(a.obj())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irIntCast:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch a.tag {
			case irValChar:
				stack = append(stack, irValue{tag: irValInt, i: int(a.char())})
			case irValInt:
				stack = append(stack, a)
			case irValDouble:
				stack = append(stack, irValue{tag: irValInt, i: int(a.f)})
			default:
				return nil
			}

		case irSubs:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if nargs == 3 {
				ei := stack[len(stack)-1]
				si := stack[len(stack)-2]
				sv := stack[len(stack)-3]
				stack = stack[:len(stack)-3]
				if sv.tag == irValString && si.tag == irValInt {
					s := sv.str()
					start := si.i
					end := ei.i
					if sv.isASCII() {
						stack = append(stack, irMakeString(s[start:end], end-start, true))
					} else {
						runes := []rune(s)
						stack = append(stack, stringToIRValue(string(runes[start:end])))
					}
				} else {
					return nil
				}
			} else {
				si := stack[len(stack)-1]
				sv := stack[len(stack)-2]
				stack = stack[:len(stack)-2]
				if sv.tag == irValString && si.tag == irValInt {
					s := sv.str()
					start := si.i
					if sv.isASCII() {
						stack = append(stack, irMakeString(s[start:], len(s)-start, true))
					} else {
						runes := []rune(s)
						stack = append(stack, stringToIRValue(string(runes[start:])))
					}
				} else {
					return nil
				}
			}

		default:
			return nil
		}
	}
	if len(stack) == 0 {
		return NIL
	}
	return stack[len(stack)-1].object()
}

// irExecTypedIV runs the typed executor and returns the result as irValue
// directly, avoiding the coretypes.Object boxing/unboxing at callSlot boundaries.
// Returns (result, true) on success, (zero, false) on failure.

// ---- typed_exec_inline.go ----
func irExecTypedIV(prog *IRProgram, initSlots []coretypes.Object) (irValue, bool) {
	result := irExecTyped(prog, initSlots)
	if result == nil {
		return irValue{}, false
	}
	return objectToIRValue(result), true
}

// irExecTypedInline executes a typed IR program with pre-filled irValue slots.
// Returns the result as irValue directly (no coretypes.Object boxing).
// Returns zero irValue on failure.
func irExecTypedInline(prog *IRProgram, slots []irValue) irValue {
	var stackBuf [32]irValue
	stack := stackBuf[:0]
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			constant, ok := runtimeExec.ProgramConstant(prog, idx)
			if !ok {
				return irValue{}
			}
			stack = append(stack, objectToIRValue(constant))
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
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: a.f + b.f})
			} else if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i + b.i})
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: a.f + float64(b.i)})
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: float64(a.i) + b.f})
			} else {
				return irValue{}
			}
		case irSub:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: a.f - b.f})
			} else if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i - b.i})
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: a.f - float64(b.i)})
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: float64(a.i) - b.f})
			} else {
				return irValue{}
			}
		case irMul:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: a.f * b.f})
			} else if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i * b.i})
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: a.f * float64(b.i)})
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: float64(a.i) * b.f})
			} else {
				return irValue{}
			}
		case irDiv:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: a.f / b.f})
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: a.f / float64(b.i)})
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: float64(a.i) / b.f})
			} else if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: float64(a.i) / float64(b.i)})
			} else {
				return irValue{}
			}
		case irLt:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i < b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f < b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f < float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) < b.f))
			} else {
				return irValue{}
			}
		case irEq:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			v, ok := irValueEq(a, b)
			if !ok {
				return irValue{}
			}
			stack = append(stack, v)
		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			cond := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !cond.truthy() {
				pc = target
			}
		case irJump:
			pc = int(code[pc])<<8 | int(code[pc+1])
		case irReturn:
			if len(stack) == 0 {
				return irValue{}
			}
			return stack[len(stack)-1]
		case irRecur:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			targetPC := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if targetPC == 0 {
				for i := n - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			} else {
				baseSlot := int(code[pc])<<8 | int(code[pc+1])
				pc += 2
				for i := n - 1; i >= 0; i-- {
					slots[baseSlot+i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			}
			pc = targetPC
			stack = stack[:0]
		case irInc:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: v.i + 1})
			} else {
				return irValue{}
			}
		case irDec:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: v.i - 1})
			} else {
				return irValue{}
			}
		case irIsZero:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag == irValInt {
				stack = append(stack, irMakeBool(v.i == 0))
			} else {
				return irValue{}
			}
		case irBitAnd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i & b.i})
			} else {
				return irValue{}
			}
		case irBitOr:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i | b.i})
			} else {
				return irValue{}
			}
		case irBitNot:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: ^a.i})
			} else {
				return irValue{}
			}
		case irBitShiftLeft:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i << uint(b.i)})
			} else {
				return irValue{}
			}
		case irBitShiftRight:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i >> uint(b.i)})
			} else {
				return irValue{}
			}
		default:
			return irValue{} // unsupported opcode — bail
		}
	}
	return irValue{}
}

// ---- typed_exec_nanbox.go ----
// ir_exec_typed_nb.go — NaN-boxed typed IR executor.
//
// Uses []uint64 stack (8 bytes per entry) instead of []irValue (32 bytes).
// Numeric operations are pure bit manipulation — zero allocation, zero copy.
// coretypes.Object operations convert at the boundary via the local nb* helpers.
//
// This is the typed executor's hot path for numeric loops.
// Falls back to nil (letting irExecTyped handle it) for unsupported patterns.

func irExecTypedNB(prog *IRProgram, initSlots []coretypes.Object) coretypes.Object {
	analysis := AnalyzeIRProgram(prog)
	// Only handle numeric-dominant programs without complex collection ops
	if !irTypedEligible(analysis) {
		return nil
	}
	// Only handle pure numeric programs — no collections, no self-calls,
	// no strings, no fn calls (which allocate []coretypes.Object args).
	if analysis.HasSelfCall || analysis.UsesString || analysis.UsesTransient ||
		analysis.UsesCollection || analysis.HasCallSlot {
		return nil
	}

	numSlots := runtimeExec.ProgramNumSlots(prog)
	var slotBuf [16]uint64
	var slots []uint64
	if numSlots <= len(slotBuf) {
		slots = slotBuf[:numSlots]
	} else {
		slots = make([]uint64, numSlots)
	}

	// coretypes.Object side-table for non-numeric values
	var objTable []coretypes.Object

	// Convert init slots
	for i := 0; i < numSlots && i < len(initSlots); i++ {
		slots[i] = coreirx.NBFromObject(initSlots[i], &objTable, IsNil)
	}
	// Pre-fill captures
	captureIdxs, captureSlots := runtimeExec.ProgramCaptureSlots(prog)
	for i, obj := range captureSlots {
		if i >= len(captureIdxs) || captureIdxs[i] < 0 || captureIdxs[i] >= len(slots) {
			return nil
		}
		slots[captureIdxs[i]] = coreirx.NBFromObject(obj, &objTable, IsNil)
	}

	// Pre-convert constants
	constants := runtimeExec.ProgramConstants(prog)
	consts := make([]uint64, len(constants))
	for i, c := range constants {
		consts[i] = coreirx.NBFromObject(c, &objTable, IsNil)
	}

	var stackBuf [32]uint64
	sp := 0
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	for pc < len(code) {
		op := code[pc]
		pc++

		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stackBuf[sp] = consts[idx]
			sp++

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stackBuf[sp] = slots[idx]
			sp++

		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			sp--
			slots[idx] = stackBuf[sp]

		case irAdd:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(a) + coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(a) + coreirx.ToFloat(b))
			}

		case irSub:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(a) - coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(a) - coreirx.ToFloat(b))
			}

		case irMul:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(a) * coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(a) * coreirx.ToFloat(b))
			}

		case irDiv:
			sp--
			stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(stackBuf[sp-1]) / coreirx.ToFloat(stackBuf[sp]))

		case irRem:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				bv := coreirx.ToInt(b)
				if bv == 0 {
					return nil
				}
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(a) % bv)
			} else {
				return nil
			}

		case irSqrt:
			stackBuf[sp-1] = coreirx.BoxDouble(math.Sqrt(coreirx.ToFloat(stackBuf[sp-1])))

		case irInc:
			v := stackBuf[sp-1]
			if coreirx.IsInt(v) {
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(v) + 1)
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(v) + 1)
			}

		case irDec:
			v := stackBuf[sp-1]
			if coreirx.IsInt(v) {
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(v) - 1)
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(v) - 1)
			}

		case irLt:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) < coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) < coreirx.ToFloat(b))
			}

		case irGte:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) >= coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) >= coreirx.ToFloat(b))
			}

		case irGt:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) > coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) > coreirx.ToFloat(b))
			}

		case irLte:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) <= coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) <= coreirx.ToFloat(b))
			}

		case irEq:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if a == b {
				stackBuf[sp-1] = coreirx.BoxBool(true)
			} else if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(false)
			} else if coreirx.IsDouble(a) || coreirx.IsDouble(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) == coreirx.ToFloat(b))
			} else {
				oa := coreirx.NBToObject(a, objTable, NIL)
				ob := coreirx.NBToObject(b, objTable, NIL)
				stackBuf[sp-1] = coreirx.BoxBool(runtimeExec.Equal(oa, ob))
			}

		case irIsZero:
			v := stackBuf[sp-1]
			if coreirx.IsInt(v) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(v) == 0)
			} else if coreirx.IsDouble(v) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToDouble(v) == 0)
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(false)
			}

		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			sp--
			if !coreirx.Truthy(stackBuf[sp]) {
				pc = target
			}

		case irJump:
			pc = int(code[pc])<<8 | int(code[pc+1])

		case irReturn:
			if sp == 0 {
				return NIL
			}
			sp--
			return coreirx.NBToObject(stackBuf[sp], objTable, NIL)

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if target != 0 {
				baseSlot := int(code[pc])<<8 | int(code[pc+1])
				pc += 2
				for i := nargs - 1; i >= 0; i-- {
					sp--
					slots[baseSlot+i] = stackBuf[sp]
				}
			} else {
				for i := nargs - 1; i >= 0; i-- {
					sp--
					slots[i] = stackBuf[sp]
				}
			}
			sp = 0
			pc = target

		// coretypes.Collection ops: convert at boundary
		case irNth:
			sp -= 2
			coll := coreirx.NBToObject(stackBuf[sp], objTable, NIL)
			idxV := stackBuf[sp+1]
			var idx int
			if coreirx.IsInt(idxV) {
				idx = coreirx.ToInt(idxV)
			} else {
				idx = int(coreirx.ToFloat(idxV))
			}
			obj, ok := runtimeExec.Nth(coll, idx)
			if !ok {
				return nil
			}
			stackBuf[sp] = coreirx.NBFromObject(obj, &objTable, IsNil)
			sp++

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnObj := coreirx.NBToObject(slots[slotIdx], objTable, NIL)
			// coretypes.Native f64 fast path
			if fnProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if nativeHelper, ok := runtimeExec.NativeHelper(fnProg); ok {
					var f64buf [4]float64
					var f64args []float64
					if nargs <= len(f64buf) {
						f64args = f64buf[:nargs]
					} else {
						f64args = make([]float64, nargs)
					}
					for i := nargs - 1; i >= 0; i-- {
						sp--
						f64args[i] = coreirx.ToFloat(stackBuf[sp])
					}
					stackBuf[sp] = coreirx.BoxDouble(nativeHelper(coreirx.Float64(f64args)))
					sp++
					continue
				}
			}
			// corecollections.Box args and call
			args := make([]coretypes.Object, nargs)
			for i := nargs - 1; i >= 0; i-- {
				sp--
				args[i] = coreirx.NBToObject(stackBuf[sp], objTable, NIL)
			}
			var result coretypes.Object
			if fnProg, ok := runtimeExec.CompileFnProgram(fnObj); ok {
				result = irExecTyped(fnProg, args)
				if result == nil {
					result = irExec(fnProg, args)
				}
				if result == nil {
					var ok bool
					result, ok = runtimeExec.CallObject(fnObj, args)
					if !ok {
						return nil
					}
				}
			} else {
				var ok bool
				result, ok = runtimeExec.CallObject(fnObj, args)
				if !ok {
					return nil
				}
			}
			stackBuf[sp] = coreirx.NBFromObject(result, &objTable, IsNil)
			sp++

		case irConj:
			sp -= 2
			coll := coreirx.NBToObject(stackBuf[sp], objTable, NIL)
			val := coreirx.NBToObject(stackBuf[sp+1], objTable, NIL)
			result, ok := runtimeExec.Conj(coll, val)
			if !ok {
				return nil
			}
			stackBuf[sp] = coreirx.NBFromObject(result, &objTable, IsNil)
			sp++

		case irCount:
			sp--
			v := stackBuf[sp]
			if !coreirx.IsObj(v) {
				return nil
			}
			count, ok := runtimeExec.Count(coreirx.NBToObject(v, objTable, NIL))
			if !ok {
				return nil
			}
			stackBuf[sp] = coreirx.BoxInt(count)
			sp++

		default:
			return nil // unsupported — fall back to irExecTyped
		}
	}

	if sp > 0 {
		return coreirx.NBToObject(stackBuf[sp-1], objTable, NIL)
	}
	return NIL
}

// ---- typed_value_accessors.go ----
// ir_value_accessors.go — typed accessors for irValue's unsafe.Pointer field.
//
// irValue stores extended data (strings, collections, objects) behind an
// unsafe.Pointer to keep the struct at 32 bytes for the numeric hot path.
// These accessors provide type-safe reads/writes.

// --- String ---

func irMakeString(s string, runeCount int, ascii bool) irValue {
	v := irValue{tag: irValString, i: runeCount, p: unsafe.Pointer(&s)}
	if ascii {
		v.f = 1
	}
	return v
}

func (v irValue) str() string {
	if v.p == nil {
		return ""
	}
	return *(*string)(v.p)
}

func (v irValue) isASCII() bool { return v.f != 0 }

// --- StringBuilder ([]byte) ---

func irMakeStringBuilder(buf []byte, runeCount int, ascii bool) irValue {
	v := irValue{tag: irValStringBuilder, i: runeCount, p: unsafe.Pointer(&buf)}
	if ascii {
		v.f = 1
	}
	return v
}

func (v irValue) bytes() []byte {
	if v.p == nil {
		return nil
	}
	return *(*[]byte)(v.p)
}

func (v *irValue) setBytes(buf []byte) {
	v.p = unsafe.Pointer(&buf)
}

func (v *irValue) setASCII(ascii bool) {
	if ascii {
		v.f = 1
	} else {
		v.f = 0
	}
}

// --- Bool ---

func irMakeBool(b bool) irValue {
	v := irValue{tag: irValBool}
	if b {
		v.i = 1
	}
	return v
}

func (v irValue) boolean() bool { return v.i != 0 }

// --- Char ---

func irMakeChar(r rune) irValue {
	return irValue{tag: irValChar, i: int(r)}
}

func (v irValue) char() rune { return rune(v.i) }

// --- StringIntMap ---

func irMakeStringIntMap(m map[string]int) irValue {
	return irValue{tag: irValStringIntMap, p: unsafe.Pointer(&m)}
}

func (v irValue) stringIntMap() map[string]int {
	if v.p == nil {
		return nil
	}
	return *(*map[string]int)(v.p)
}

func (v *irValue) setStringIntMap(m map[string]int) {
	v.p = unsafe.Pointer(&m)
}

// --- IntVector ---

func irMakeIntVector(iv []int) irValue {
	return irValue{tag: irValIntVector, p: unsafe.Pointer(&iv)}
}

func (v irValue) intVec() []int {
	if v.p == nil {
		return nil
	}
	return *(*[]int)(v.p)
}

func (v *irValue) setIntVec(iv []int) {
	v.p = unsafe.Pointer(&iv)
}

// --- coretypes.Object ---

func irMakeObject(obj coretypes.Object) irValue {
	// For common concrete pointer types, store directly to avoid
	// allocating an coretypes.Object interface box. Use i field as sub-tag.
	switch v := obj.(type) {
	case *corecollections.ArrayVector:
		return irValue{tag: irValObject, i: 1, p: unsafe.Pointer(v)}
	case *coretypes.TransientVector:
		return irValue{tag: irValObject, i: 2, p: unsafe.Pointer(v)}
	case *Fn:
		return irValue{tag: irValObject, i: 3, p: unsafe.Pointer(v)}
	default:
		p := new(coretypes.Object)
		*p = obj
		return irValue{tag: irValObject, i: 0, p: unsafe.Pointer(p)}
	}
}

func (v irValue) obj() coretypes.Object {
	if v.p == nil {
		return NIL
	}
	switch v.i {
	case 1:
		return (*corecollections.ArrayVector)(v.p)
	case 2:
		return (*coretypes.TransientVector)(v.p)
	case 3:
		return (*Fn)(v.p)
	default:
		return *(*coretypes.Object)(v.p)
	}
}

// ---- typed_values.go ----
// ir_typed.go — experimental typed IR executor (v2).
//
// This is the first incremental step away from the boxed []coretypes.Object stack used by
// irExec. It is intentionally small and gated: primitive/string-only loops can
// be executed with tagged values, while unsupported opcodes return nil and let
// the normal IR/tree path handle them.

type irValueTag byte

const (
	irValObject irValueTag = iota
	irValInt
	irValDouble
	irValBool
	irValChar
	irValString
	irValStringBuilder
	irValStringIntMap
	irValIntVector
	irValNil
	irValKeyword
	irValCursor // StringCursor pointer in obj field
)

// irValue is the tagged value for the typed IR executor.
// Layout: 32 bytes for the compact numeric path.
// String/collection data is stored behind an unsafe.Pointer to avoid
// bloating the struct for the common numeric case.
type irValue struct {
	tag irValueTag
	i   int            // int value, bool (0/1), rune, rune count for strings
	f   float64        // double value, or ASCII flag (nonzero = ASCII) for strings
	p   unsafe.Pointer // -> string | []byte | map[string]int | []int | coretypes.Object
}

func irTypedEligible(a coreirx.Analysis) bool {
	if a.NumOps == 0 || a.UsesTransient {
		return false
	}
	// Call-slot loops: allow if numeric-only or numeric+generic-nth
	if a.HasCallSlot {
		return !a.UsesString && !a.HasMapOps && (!a.UsesCollection || a.HasGenericNth)
	}
	// coretypes.Collection programs with nth but NO assoc (read-only vector access)
	if a.UsesCollection && a.HasGenericNth && !a.HasMapOps && !a.UsesString && !a.HasAssoc {
		return true
	}
	// coretypes.Collection programs with assoc: prefer boxed executor (has transient support)
	if a.UsesCollection && a.HasGenericNth && a.HasAssoc && !a.HasMapOps && !a.UsesString {
		return false
	}
	if a.UsesCollection && (a.HasMapOps || !a.HasGenericNth) {
		if corert.IRTypedMapEnabled() && a.HasMapOps && a.UsesString {
			return true
		}
		// Self-recursive tree builders/walkers (binary-trees pattern)
		if a.HasSelfCall && !a.HasMapOps && !a.UsesString {
			return true
		}
		return corert.IRTypedVecEnabled() && a.UsesCollection && !a.UsesString && !a.HasMapOps
	}
	// Accept: pure numeric loops (no strings, no collections, no call-slots)
	if !a.UsesString && !a.UsesCollection && !a.HasCallSlot {
		return true
	}
	return a.UsesString || a.SuggestedPath == "typed-ir-string-candidate" || a.SuggestedPath == "typed-ir-generic-string-nth-candidate"
}

func stringToIRValue(s string) irValue {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			return irMakeString(s, utf8.RuneCountInString(s), false)
		}
	}
	return irMakeString(s, len(s), ascii)
}

func objectToIRValue(obj coretypes.Object) irValue {
	switch v := obj.(type) {
	case coretypes.Int:
		return irValue{tag: irValInt, i: v.I}
	case coretypes.Double:
		return irValue{tag: irValDouble, f: v.D}
	case coretypes.Boolean:
		return irMakeBool(v.B)
	case coretypes.Char:
		return irMakeChar(v.Ch)
	case coretypes.String:
		return stringToIRValue(v.S)
	case *corecollections.ArrayVector:
		if corert.IRTypedVecEnabled() {
			iv := make([]int, len(v.Arr))
			for i, obj := range v.Arr {
				x, ok := obj.(coretypes.Int)
				if !ok {
					return irMakeObject(obj)
				}
				iv[i] = x.I
			}
			return irMakeIntVector(iv)
		}
	case *corecollections.ArrayMap:
		if v.Count() == 0 {
			return irMakeStringIntMap(make(map[string]int))
		}
	case *corecollections.HashMap:
		if v.Count() == 0 {
			return irMakeStringIntMap(make(map[string]int))
		}
	case Nil:
		return irValue{tag: irValNil}
	case coretypes.Keyword:
		return irValue{tag: irValKeyword, p: unsafe.Pointer(v.NameKey())}
	case *StringCursor:
		return irValue{tag: irValCursor, p: unsafe.Pointer(v)}
	default:
		return irMakeObject(obj)
	}
	return irMakeObject(obj)
}

func (v irValue) object() coretypes.Object {
	switch v.tag {
	case irValInt:
		return coretypes.Int{I: v.i}
	case irValDouble:
		return coretypes.Double{D: v.f}
	case irValBool:
		return coretypes.Boolean{B: v.boolean()}
	case irValChar:
		return coretypes.Char{Ch: v.char()}
	case irValString:
		return coretypes.String{S: v.str()}
	case irValStringBuilder:
		return coretypes.String{S: string(v.bytes())}
	case irValStringIntMap:
		res := corecollections.EmptyArrayMap()
		for k, v := range v.stringIntMap() {
			res.Add(coretypes.String{S: k}, coretypes.Int{I: v})
		}
		return res
	case irValIntVector:
		arr := make([]coretypes.Object, len(v.intVec()))
		for i, x := range v.intVec() {
			arr[i] = coretypes.Int{I: x}
		}
		return runtimeExec.BuildVector(arr)
	case irValNil:
		return NIL
	case irValKeyword:
		return keywordObjectFromName((*string)(v.p))
	case irValCursor:
		return (*StringCursor)(v.p)
	default:
		if v.obj() == nil {
			return NIL
		}
		return v.obj()
	}
}

func (v irValue) truthy() bool {
	switch v.tag {
	case irValBool:
		return v.boolean()
	case irValNil:
		return false
	default:
		return true
	}
}

func irValueToString(v irValue) string {
	switch v.tag {
	case irValString:
		return v.str()
	case irValStringBuilder:
		return string(v.bytes())
	case irValChar:
		return charToStringFast(v.char())
	case irValNil:
		return ""
	case irValInt:
		return strconv.Itoa(v.i)
	case irValDouble:
		return strconv.FormatFloat(v.f, 'g', -1, 64)
	case irValBool:
		if v.boolean() {
			return "true"
		}
		return "false"
	default:
		return v.object().ToString(false)
	}
}

func irValueStringKey(v irValue) (string, bool) {
	switch v.tag {
	case irValString:
		return v.str(), true
	case irValStringBuilder:
		return string(v.bytes()), true
	case irValChar:
		return charToStringFast(v.char()), true
	default:
		return "", false
	}
}

func irStringRuneCount(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return utf8.RuneCountInString(s)
		}
	}
	return len(s)
}

func irValueEq(a, b irValue) (irValue, bool) {
	if a.tag == b.tag {
		switch a.tag {
		case irValInt:
			return irMakeBool(a.i == b.i), true
		case irValDouble:
			return irMakeBool(a.f == b.f), true
		case irValBool:
			return irMakeBool(a.boolean() == b.boolean()), true
		case irValChar:
			return irMakeBool(a.char() == b.char()), true
		case irValString:
			return irMakeBool(a.str() == b.str()), true
		case irValStringBuilder:
			return irMakeBool(string(a.bytes()) == string(b.bytes())), true
		case irValNil:
			return irMakeBool(true), true
		case irValKeyword:
			// Keywords are interned — pointer equality on name
			return irMakeBool(a.p == b.p), true
		}
	}
	if a.tag == irValInt && b.tag == irValDouble {
		return irMakeBool(float64(a.i) == b.f), true
	}
	if a.tag == irValDouble && b.tag == irValInt {
		return irMakeBool(a.f == float64(b.i)), true
	}
	return irMakeBool(runtimeExec.Equal(a.object(), b.object())), true
}

// keywordObjectCache caches Keyword Objects by name pointer to avoid
// repeated heap allocation when converting irValKeyword → coretypes.Object.
var keywordObjectCache sync.Map // *string → coretypes.Object (Keyword)

func keywordObjectFromName(name *string) coretypes.Object {
	if v, ok := keywordObjectCache.Load(name); ok {
		return v.(coretypes.Object)
	}
	kw := coretypes.MakeKeywordFromKeys(nil, name)
	// Store as coretypes.Object interface to avoid re-boxing
	var obj coretypes.Object = kw
	keywordObjectCache.Store(name, obj)
	return obj
}

// ---- wasm_exec_runtime.go ----
// wasm_runtime.go — wazero-based WASM execution engine.
// Compiles WASM modules and caches them. Handles coretypes.Object ↔ WASM i64 conversion.

// WasmProgram is a compiled, ready-to-execute WASM module.
type WasmProgram struct {
	mod        api.Module
	execFn     api.Function
	useFloat   bool
	hasImports bool
	constants  []coretypes.Object // pre-stored constants for handle references
	bytes      []byte             // raw wasm module for export/debugging
}

var (
	wasmRT     wazero.Runtime
	wasmRTOnce sync.Once
	wasmCache  sync.Map // map[*IRProgram]*WasmProgram
	wasmFail   = &WasmProgram{}
)

func getWasmRT() wazero.Runtime {
	wasmRTOnce.Do(func() {
		cache := wazero.NewCompilationCache()
		wasmRT = wazero.NewRuntimeWithConfig(context.Background(),
			wazero.NewRuntimeConfig().WithCompilationCache(cache))
		// Register host functions for collection operations
		registerWasmHost(wasmRT)
	})
	return wasmRT
}

// wasmGetCached retrieves or compiles a WASM program for an IR program.
func wasmGetCached(prog *IRProgram) *WasmProgram {
	if v, ok := wasmCache.Load(prog); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFail {
			return nil
		}
		return wp
	}
	wp := wasmCompile(prog)
	if wp == nil {
		wasmCache.Store(prog, wasmFail)
		return nil
	}
	wasmCache.Store(prog, wp)
	return wp
}

// wasmCompile translates IR → WASM binary → wazero compiled module.
func closeWasmModule(ctx context.Context, mod api.Module) {
	if err := mod.Close(ctx); err != nil {
		fmt.Fprintln(Stderr, "wasm module close error:", err)
	}
}

func wasmCompile(prog *IRProgram) *WasmProgram {
	// Try pure-numeric path first (faster, no imports needed)
	bin := irToWasm(prog)
	// TODO: enable imports path once collection handle ABI/control-flow is fully validated.
	// if bin == nil {
	// 	bin = irToWasmWithImports(prog)
	// }
	if bin == nil {
		return nil
	}

	rt := getWasmRT()
	ctx := context.Background()

	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return nil
	}

	cfg := wazero.NewModuleConfig().WithName(corewasm.NextWasmModuleName())
	mod, err := rt.InstantiateModule(ctx, compiled, cfg)
	if err != nil {
		return nil
	}

	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		closeWasmModule(ctx, mod)
		return nil
	}
	model := runtimeExec.ProgramModel(prog)
	if model == nil {
		closeWasmModule(ctx, mod)
		return nil
	}

	wp := &WasmProgram{
		mod:        mod,
		execFn:     execFn,
		useFloat:   corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0),
		hasImports: !corewasm.Eligible(model.Code),
		constants:  runtimeExec.ProgramConstants(prog),
		bytes:      append([]byte(nil), bin...),
	}
	return wp
}

func wasmExec(wp *WasmProgram, slots []coretypes.Object) coretypes.Object {
	// Create object table for this execution
	table := corewasm.NewObjectTable(NIL)

	// Pre-populate with IR program constants (for handle references)
	if wp.hasImports && len(wp.constants) > 0 {
		for _, c := range wp.constants {
			table.Store(c)
		}
	}

	var stackBuf [16]uint64
	var stack []uint64
	if len(slots) <= len(stackBuf) {
		stack = stackBuf[:len(slots)]
	} else {
		stack = make([]uint64, len(slots))
	}
	for i, s := range slots {
		switch v := s.(type) {
		case coretypes.Int:
			if wp.useFloat {
				stack[i] = math.Float64bits(float64(v.I))
			} else {
				stack[i] = uint64(v.I)
			}
		case coretypes.Double:
			if wp.useFloat {
				stack[i] = math.Float64bits(v.D)
			} else {
				return nil
			}
		default:
			stack[i] = table.Store(s)
		}
	}

	ctx := corewasm.WithObjectTable(context.Background(), table)
	if err := wp.execFn.CallWithStack(ctx, stack); err != nil {
		return nil
	}

	r := stack[0]
	if wp.useFloat {
		return coretypes.Double{D: math.Float64frombits(r)}
	}
	// Check if result is a handle
	if corewasm.IsHandle(r) {
		return table.Load(r)
	}
	return corewasm.RawIntObject(r)
}

// Ensure math import is used
var _ = math.Float64bits

// ---- runtime_execution_contract.go ----

// RuntimeExecutionAdapter is the narrow root-owned runtime surface that future
// extracted IR executors should target instead of reaching through all of core.
// It is intentionally small and grows only when contract tests justify a new
// operation.
type RuntimeExecutionAdapter struct{}

var runtimeExec RuntimeExecutionAdapter

func (RuntimeExecutionAdapter) Errorf(format string, args ...any) coretypes.Error {
	return RT.NewError(fmt.Sprintf(format, args...))
}

func (r RuntimeExecutionAdapter) Throw(obj coretypes.Object) {
	panic(r.Errorf("%s", obj.ToString(false)))
}

func (RuntimeExecutionAdapter) Equal(a coretypes.Object, b coretypes.Object) bool {
	return a.Equals(b)
}

func (RuntimeExecutionAdapter) ApplyCaptureSlots(slots []coretypes.Object, idxs []int, values []coretypes.Object) bool {
	if len(idxs) != len(values) {
		return false
	}
	for i, obj := range values {
		idx := idxs[i]
		if idx < 0 || idx >= len(slots) {
			return false
		}
		slots[idx] = obj
	}
	return true
}

func (RuntimeExecutionAdapter) ApplyTypedCaptureSlots(slots []irValue, idxs []int, values []coretypes.Object) bool {
	if len(idxs) != len(values) {
		return false
	}
	for i, obj := range values {
		idx := idxs[i]
		if idx < 0 || idx >= len(slots) {
			return false
		}
		slots[idx] = objectToIRValue(obj)
	}
	return true
}

func (r RuntimeExecutionAdapter) PrepareCallSlots(prog *IRProgram, args []coretypes.Object, env *LocalEnv) []coretypes.Object {
	if prog == nil || len(prog.captureKeys) == 0 {
		return args
	}
	full := make([]coretypes.Object, prog.numSlots)
	copy(full, args)
	r.InstallEnvCaptures(prog, full, env)
	return full
}

func (RuntimeExecutionAdapter) InstallEnvCaptures(prog *IRProgram, slots []coretypes.Object, env *LocalEnv) {
	if prog == nil {
		return
	}
	for ci, ck := range prog.captureKeys {
		if ci >= len(prog.captureSlotIdxs) {
			return
		}
		idx := prog.captureSlotIdxs[ci]
		if idx < 0 || idx >= len(slots) {
			continue
		}
		for e := env; e != nil; e = e.parent {
			if ck.index < len(e.bindings) {
				slots[idx] = e.bindings[ck.index]
				break
			}
		}
	}
}

func (RuntimeExecutionAdapter) InstallTypedEnvCaptures(prog *IRProgram, slots []irValue, env *LocalEnv) {
	if prog == nil {
		return
	}
	for ci, ck := range prog.captureKeys {
		if ci >= len(prog.captureSlotIdxs) {
			return
		}
		idx := prog.captureSlotIdxs[ci]
		if idx < 0 || idx >= len(slots) {
			continue
		}
		for e := env; e != nil; e = e.parent {
			if ck.index < len(e.bindings) {
				slots[idx] = objectToIRValue(e.bindings[ck.index])
				break
			}
		}
	}
}

func (RuntimeExecutionAdapter) MakeFn(fnExpr *FnExpr, slots []coretypes.Object) coretypes.Object {
	fnEnv := &LocalEnv{bindings: make([]coretypes.Object, len(slots))}
	copy(fnEnv.bindings, slots)
	return &Fn{fnExpr: fnExpr, env: fnEnv}
}

func (RuntimeExecutionAdapter) CallArgs(argsSeq coretypes.Object) ([]coretypes.Object, bool) {
	seqable, ok := argsSeq.(coretypes.Seqable)
	if !ok {
		return nil, false
	}
	seq := seqable.Seq()
	if seq == nil {
		return nil, true
	}
	return corecollections.ToSlice(seq), true
}

func (RuntimeExecutionAdapter) CallObject(fnObj coretypes.Object, args []coretypes.Object) (coretypes.Object, bool) {
	callable, ok := fnObj.(coretypes.Callable)
	if !ok {
		return nil, false
	}
	return callable.Call(args), true
}

func (adapter RuntimeExecutionAdapter) CallObjectWithSyntheticCallExpr(fnObj coretypes.Object, args []coretypes.Object) (coretypes.Object, bool) {
	grt := currentGRT()
	prevExpr := grt.currentExpr
	grt.currentExpr = &CallExpr{}
	defer func() { grt.currentExpr = prevExpr }()
	return adapter.CallObject(fnObj, args)
}

func (RuntimeExecutionAdapter) HasMutableSlotCandidate(slots []coretypes.Object) bool {
	for _, s := range slots {
		switch s.(type) {
		case *corecollections.ArrayVector, *corecollections.ArrayMap, *corecollections.HashMap, coretypes.String:
			return true
		}
	}
	return false
}

func (RuntimeExecutionAdapter) MutableSlotObject(obj coretypes.Object, escapeInfo *EscapeInfo, slot int) coretypes.Object {
	if escapeInfo == nil || slot < 0 || slot >= len(escapeInfo.SafeMutableSlots) || !escapeInfo.SafeMutableSlots[slot] {
		return obj
	}
	switch v := obj.(type) {
	case *corecollections.ArrayVector:
		return coretypes.ToTransient(v.Arr)
	case *corecollections.ArrayMap:
		return coretypes.MapToTransient(v)
	case *corecollections.HashMap:
		return coretypes.MapToTransient(v)
	case coretypes.String:
		if !corert.IRStringBuilderDisabled() && slot < len(escapeInfo.StringPrependSlots) {
			builder := slot < len(escapeInfo.StringBuilderSlots) && escapeInfo.StringBuilderSlots[slot]
			prepend := escapeInfo.StringPrependSlots[slot]
			if (corert.IRStringBuilderForce() && (builder || prepend)) || (!corert.IRStringBuilderForce() && prepend) {
				return NewTransientString(v)
			}
		}
	}
	return obj
}

func (RuntimeExecutionAdapter) PersistentResult(result coretypes.Object) coretypes.Object {
	switch v := result.(type) {
	case *coretypes.TransientVector:
		return v.ToPersistent()
	case *coretypes.TransientMap:
		return v.ToPersistent()
	case *TransientString:
		return v.ToPersistent()
	default:
		return result
	}
}

func (RuntimeExecutionAdapter) Get(coll coretypes.Object, key coretypes.Object, def coretypes.Object) coretypes.Object {
	if g, ok := coll.(coretypes.Gettable); ok {
		if ok, v := g.Get(key); ok {
			return v
		}
	}
	return def
}

func (RuntimeExecutionAdapter) Assoc(coll coretypes.Object, key coretypes.Object, val coretypes.Object) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *coretypes.TransientVector:
		return c.AssocInPlace(key, val), true
	case *coretypes.TransientMap:
		return c.AssocInPlace(key, val), true
	case coretypes.Associative:
		return c.Assoc(key, val), true
	default:
		return nil, false
	}
}

// stringNthFast returns the i-th rune of s with an ASCII-prefix fast path.
//
// Joker's string indexing is by rune index. For ASCII prefixes, byte and rune
// offsets are identical, which covers the common CLBG/gi text-processing hot
// path without changing Unicode semantics. If a non-ASCII byte appears before
// the requested index, this falls back to the Unicode-correct range walk.
func stringNthFast(s string, i int) coretypes.Object {
	if i < 0 {
		panic(RT.NewError(fmt.Sprintf("Negative index: %d", i)))
	}
	if r, length, ok := corestr.NthRune(s, i); ok {
		return coretypes.Char{Ch: r}
	} else {
		panic(RT.NewError(fmt.Sprintf("Index %d exceeds string's length %d", i, length)))
	}
}

func (RuntimeExecutionAdapter) Nth(coll coretypes.Object, idx int) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *corecollections.ArrayVector:
		if idx >= 0 && idx < len(c.Arr) {
			return c.Arr[idx], true
		}
	case *coretypes.TransientVector:
		if idx >= 0 && idx < len(c.Arr) {
			return c.Arr[idx], true
		}
	case coretypes.String:
		return stringNthFast(c.S, idx), true
	case coretypes.Indexed:
		return c.Nth(idx), true
	}
	return nil, false
}

func (RuntimeExecutionAdapter) Conj(coll coretypes.Object, val coretypes.Object) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *coretypes.TransientVector:
		return c.ConjInPlace(val), true
	case coretypes.Conjable:
		return c.Conj(val), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) First(coll coretypes.Object) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *corecollections.ArrayVector:
		if len(c.Arr) > 0 {
			return c.Arr[0], true
		}
		return NIL, true
	case *coretypes.TransientVector:
		if len(c.Arr) > 0 {
			return c.Arr[0], true
		}
		return NIL, true
	case coretypes.Seqable:
		s := c.Seq()
		if s == nil || s.IsEmpty() {
			return NIL, true
		}
		return s.First(), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) BuildVector(items []coretypes.Object) coretypes.Object {
	arr := make([]coretypes.Object, len(items))
	copy(arr, items)
	return &corecollections.ArrayVector{Arr: arr}
}

func (RuntimeExecutionAdapter) ToTransient(coll coretypes.Object) (coretypes.Object, bool) {
	if av, ok := coll.(*corecollections.ArrayVector); ok {
		return coretypes.ToTransient(av.Arr), true
	}
	return nil, false
}

func (RuntimeExecutionAdapter) AssocBang(coll coretypes.Object, key coretypes.Object, val coretypes.Object) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *coretypes.TransientVector:
		return c.AssocInPlace(key, val), true
	case *coretypes.TransientMap:
		return c.AssocInPlace(key, val), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) ToPersistent(coll coretypes.Object) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *coretypes.TransientVector:
		return c.ToPersistent(), true
	case *coretypes.TransientMap:
		return c.ToPersistent(), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) Str1(obj coretypes.Object) coretypes.Object {
	switch v := obj.(type) {
	case Nil:
		return coretypes.String{S: ""}
	case coretypes.String:
		return v
	case coretypes.Char:
		return charToStringObjectFast(v.Ch)
	default:
		return coretypes.String{S: obj.ToString(false)}
	}
}

func (RuntimeExecutionAdapter) Str2(a coretypes.Object, b coretypes.Object) coretypes.Object {
	switch av := a.(type) {
	case *TransientString:
		switch bv := b.(type) {
		case coretypes.Char:
			return av.AppendChar(bv.Ch)
		case coretypes.String:
			return av.AppendString(bv.S)
		default:
			return av.AppendString(b.ToString(false))
		}
	case coretypes.String:
		switch bv := b.(type) {
		case coretypes.Char:
			return coretypes.String{S: av.S + charToStringFast(bv.Ch)}
		case coretypes.String:
			return coretypes.String{S: av.S + bv.S}
		case *TransientString:
			return bv.PrependString(av.S)
		default:
			return coretypes.String{S: av.S + b.ToString(false)}
		}
	case coretypes.Char:
		if bv, ok := b.(*TransientString); ok {
			return bv.PrependChar(av.Ch)
		}
		return coretypes.String{S: charToStringFast(av.Ch) + b.ToString(false)}
	default:
		return coretypes.String{S: a.ToString(false) + b.ToString(false)}
	}
}

func (RuntimeExecutionAdapter) Count(obj coretypes.Object) (int, bool) {
	switch v := obj.(type) {
	case *TransientString:
		return v.Count(), true
	case coretypes.Counted:
		return v.Count(), true
	default:
		return 0, false
	}
}

func (adapter RuntimeExecutionAdapter) NthASCIIStringConst(prog *IRProgram, constIdx int, idx int) (coretypes.Object, bool) {
	constant, ok := adapter.ProgramConstant(prog, constIdx)
	if !ok {
		return nil, false
	}
	s, ok := constant.(coretypes.String)
	if !ok || idx < 0 || idx >= len(s.S) {
		return nil, false
	}
	return coretypes.Char{Ch: rune(s.S[idx])}, true
}

func (RuntimeExecutionAdapter) CursorChar(obj coretypes.Object) (coretypes.Object, bool) {
	cur, ok := obj.(*StringCursor)
	if !ok {
		return nil, false
	}
	if r := cur.Char(); r >= 0 {
		return coretypes.Char{Ch: r}, true
	}
	return NIL, true
}

func (RuntimeExecutionAdapter) CursorNext(obj coretypes.Object) (coretypes.Object, bool) {
	cur, ok := obj.(*StringCursor)
	if !ok {
		return nil, false
	}
	return cur.Next(), true
}

func (RuntimeExecutionAdapter) CursorDone(obj coretypes.Object) (coretypes.Object, bool) {
	cur, ok := obj.(*StringCursor)
	if !ok {
		return nil, false
	}
	return coretypes.Boolean{B: cur.Done()}, true
}

func (RuntimeExecutionAdapter) MarkTypedExecutionFailed(prog *IRProgram) {
	if prog != nil {
		prog.typedFailed = true
	}
}

func (RuntimeExecutionAdapter) MarkBoxedExecutionFailed(prog *IRProgram) {
	if prog != nil {
		prog.execFailed = true
	}
}

func (RuntimeExecutionAdapter) ProgramNumSlots(prog *IRProgram) int {
	if prog == nil {
		return 0
	}
	return prog.numSlots
}

func (RuntimeExecutionAdapter) ProgramCode(prog *IRProgram) []byte {
	if prog == nil {
		return nil
	}
	return prog.code
}

func (RuntimeExecutionAdapter) ProgramModel(prog *IRProgram) *coreir.Program {
	if prog == nil {
		return nil
	}
	return prog.neutralModel()
}

func (RuntimeExecutionAdapter) ProgramConstant(prog *IRProgram, idx int) (coretypes.Object, bool) {
	if prog == nil || idx < 0 || idx >= len(prog.constants) {
		return nil, false
	}
	return prog.constants[idx], true
}

func (RuntimeExecutionAdapter) ProgramConstants(prog *IRProgram) []coretypes.Object {
	if prog == nil {
		return nil
	}
	return prog.constants
}

func (RuntimeExecutionAdapter) ProgramFnExpr(prog *IRProgram, idx int) (*FnExpr, bool) {
	if prog == nil || idx < 0 || idx >= len(prog.fnExprs) {
		return nil, false
	}
	return prog.fnExprs[idx], true
}

func (RuntimeExecutionAdapter) FnProgram(fnObj coretypes.Object) (*IRProgram, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	if fn.irProg != nil {
		if fn.irProg == irCompileFailed {
			return nil, false
		}
		return fn.irProg, true
	}
	prog := irGetFnProg(fn)
	return prog, prog != nil
}

func (RuntimeExecutionAdapter) CompileFnProgram(fnObj coretypes.Object) (*IRProgram, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	prog := irCompileFn(fn)
	return prog, prog != nil
}

func (RuntimeExecutionAdapter) FnWasmExec(fnObj coretypes.Object, args []coretypes.Object) (coretypes.Object, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	wp := wasmGetFn(fn)
	if wp == nil {
		return nil, false
	}
	result := wasmExec(wp, args)
	return result, result != nil
}

func (adapter RuntimeExecutionAdapter) FnCallSlots(fnObj coretypes.Object, prog *IRProgram, args []coretypes.Object) ([]coretypes.Object, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	return adapter.PrepareCallSlots(prog, args, fn.env), true
}

func (adapter RuntimeExecutionAdapter) InstallFnTypedEnvCaptures(fnObj coretypes.Object, prog *IRProgram, slots []irValue) bool {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return false
	}
	adapter.InstallTypedEnvCaptures(prog, slots, fn.env)
	return true
}

func (RuntimeExecutionAdapter) ObjectsFromTypedValues(values []irValue, buf []coretypes.Object) []coretypes.Object {
	var out []coretypes.Object
	if len(values) <= len(buf) {
		out = buf[:len(values)]
	} else {
		out = make([]coretypes.Object, len(values))
	}
	for i, v := range values {
		out[i] = v.object()
	}
	return out
}

func (adapter RuntimeExecutionAdapter) DispatchArityProgram(prog *IRProgram, nargs int) *IRProgram {
	if prog == nil {
		return nil
	}
	if prog.arityPrograms == nil {
		if prog.variadicMinArgs > 0 && nargs < prog.variadicMinArgs {
			return nil
		}
		return prog
	}
	if sub, ok := prog.arityPrograms[nargs]; ok {
		return sub
	}
	if prog.variadicProg != nil && nargs >= prog.variadicMinArgs {
		return prog.variadicProg
	}
	return nil
}

func (RuntimeExecutionAdapter) ProgramHasCaptureSlots(prog *IRProgram) bool {
	return prog != nil && len(prog.captureSlots) > 0
}

func (RuntimeExecutionAdapter) ProgramEscapeInfo(prog *IRProgram) *EscapeInfo {
	if prog == nil {
		return nil
	}
	if prog.escapeInfo == nil {
		prog.escapeInfo = analyzeEscapes(prog)
	}
	return prog.escapeInfo
}

func (RuntimeExecutionAdapter) ProgramAnalysis(prog *IRProgram) coreir.Analysis {
	return AnalyzeIRProgram(prog)
}

func (adapter RuntimeExecutionAdapter) ApplyProgramCaptureSlots(prog *IRProgram, slots []coretypes.Object) bool {
	if prog == nil {
		return false
	}
	return adapter.ApplyCaptureSlots(slots, prog.captureSlotIdxs, prog.captureSlots)
}

func (adapter RuntimeExecutionAdapter) ApplyProgramTypedCaptureSlots(prog *IRProgram, slots []irValue) bool {
	if prog == nil {
		return false
	}
	return adapter.ApplyTypedCaptureSlots(slots, prog.captureSlotIdxs, prog.captureSlots)
}

func (adapter RuntimeExecutionAdapter) ClearTypedNonCaptureSlots(prog *IRProgram, slots []irValue, keepArgs int) bool {
	if keepArgs < 0 || keepArgs > len(slots) {
		return false
	}
	if prog != nil && prog.captureSlotSet != nil {
		if len(prog.captureSlotSet) < len(slots) {
			return false
		}
		for i := keepArgs; i < len(slots); i++ {
			if !prog.captureSlotSet[i] {
				slots[i] = irValue{}
			}
		}
		return true
	}
	for i := keepArgs; i < len(slots); i++ {
		slots[i] = irValue{}
	}
	if prog == nil || len(prog.captureSlots) == 0 {
		return true
	}
	return adapter.ApplyProgramTypedCaptureSlots(prog, slots)
}

func (RuntimeExecutionAdapter) ProgramCaptureSlots(prog *IRProgram) ([]int, []coretypes.Object) {
	if prog == nil {
		return nil, nil
	}
	return prog.captureSlotIdxs, prog.captureSlots
}

func (RuntimeExecutionAdapter) CanExecuteIR(prog *IRProgram) bool {
	return prog != nil && !prog.execFailed
}

func (RuntimeExecutionAdapter) CanExecuteTypedIR(prog *IRProgram) bool {
	return prog != nil && !prog.typedFailed && !prog.execFailed
}

func (RuntimeExecutionAdapter) HasNativeHelper(prog *IRProgram) bool {
	return prog != nil && prog.nativeHelper != nil
}

func (RuntimeExecutionAdapter) NativeHelper(prog *IRProgram) (nativeF64Fn, bool) {
	if prog == nil || prog.nativeHelper == nil {
		return nil, false
	}
	return prog.nativeHelper, true
}

func (RuntimeExecutionAdapter) InstallNativeHelper(prog *IRProgram, helper nativeF64Fn) {
	if prog != nil {
		prog.nativeHelper = helper
		prog.nativeChecked = true
	}
}

func (RuntimeExecutionAdapter) NativeHelperChecked(prog *IRProgram) bool {
	return prog != nil && prog.nativeChecked
}

func (RuntimeExecutionAdapter) CanTryMemNth(prog *IRProgram) bool {
	return prog != nil && !prog.memNthFailed
}

func (RuntimeExecutionAdapter) MarkMemNthFailed(prog *IRProgram) {
	if prog != nil {
		prog.memNthFailed = true
	}
}
