package core

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

	if cached, ok := irFnCache.Load(&arity); ok {
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
			return prog
		}
	}
	irFnCache.Store(&arity, irCompileFailed)
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
	}
	// Eagerly compile native f64 helper if eligible
	prog.nativeHelper = irCompileNativeHelper(prog)
	prog.nativeChecked = true
	// Cache at arity level. For fns with captures, store a "template"
	// program. The captureSlots are resolved per-instance but the bytecode
	// and captureKeys are shared.
	irFnCache.Store(&arity, prog)
	return prog
}

// irCompileMultiArity compiles a multi-arity fn into an IRProgram with
// arityPrograms map for dispatch by arg count.
func irCompileMultiArity(fn *Fn) *IRProgram {
	// Check cache using first arity
	firstArity := fn.fnExpr.arities[0]
	if cached, ok := irFnCache.Load(&firstArity); ok {
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
			irFnCache.Store(&firstArity, irCompileFailed)
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
	wrapper := &IRProgram{
		arityPrograms:   programs,
		variadicProg:    varProg,
		variadicMinArgs: varMinArgs,
	}
	irFnCache.Store(&firstArity, wrapper)
	return wrapper
}

// irCompileVariadicFn compiles a variadic fn (fn [x & rest] ...).
// The rest parameter is packed into a vector from remaining args.
func irCompileVariadicFn(fn *Fn) *IRProgram {
	va := *fn.fnExpr.variadic
	firstArity := va // use variadic as the cache key stand-in

	if cached, ok := irFnCache.Load(&firstArity); ok {
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
	if prog == nil {
		irFnCache.Store(&firstArity, irCompileFailed)
		return nil
	}
	// Mark as variadic so the executor knows to pack rest args
	prog.variadicMinArgs = len(va.args) - 1 // exclude the & rest param from required count
	irFnCache.Store(&firstArity, prog)
	return prog
}
