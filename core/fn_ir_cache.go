package core

import "sync/atomic"

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
		case Int:
			initVals[i] = float64(lv.I)
		case Double:
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
					case Int:
						caps[ci] = capInfo{constVal: float64(v.I)}
						resolved = true
					case Double:
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
