package core

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
	// Resolve captures from fn's closure environment.
	// Captures reference parse-time frames, but fn.env holds the actual
	// runtime values. Walk the env chain to find matching bindings.
	if len(c.captureKeys) > 0 {
		capSlots := make([]Object, len(c.captureKeys))
		capIdxs := make([]int, len(c.captureKeys))
		allResolved := true
		for ci, ck := range c.captureKeys {
			capIdxs[ci] = c.bindingMap[ck]
			resolved := false
			e := fn.env
			for e != nil {
				if ck.index < len(e.bindings) {
					capSlots[ci] = e.bindings[ck.index]
					resolved = true
					break
				}
				e = e.parent
			}
			if !resolved {
				allResolved = false
				break
			}
		}
		if !allResolved {
			
			return nil
		}
		c.captureSlots = capSlots
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
	// Don't cache in irFnCache if we have closure captures — each fn
	// instance may have different env bindings for the same arity.
	if len(prog.captureSlots) == 0 {
		irFnCache.Store(&arity, prog)
	}
	return prog
}
