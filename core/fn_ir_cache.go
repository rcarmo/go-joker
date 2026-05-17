package core

import (
	coreir "github.com/rcarmo/go-joker/core/ir"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
	"sync"
	"sync/atomic"
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
		stableArgs := make([]coretypes.Object, len(args))
		copy(stableArgs, args)
		args = stableArgs
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
	if fn.fnExpr.self.name != nil {
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
	irToTransient    = coreir.ToTransient    // pop 1 (ArrayVector), push TransientVector
	irAssocBang      = coreir.AssocBang      // pop 3 (tv, key, val), mutate in place, push tv
	irToPersistent   = coreir.ToPersistent   // pop 1 (TransientVector), push ArrayVector
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
