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
func irGetFnProg(fn *Fn) *IRProgram {
	if atomic.LoadUint32(&fn.irProgOnce) == 1 {
		return irGetCachedFnProg(fn)
	}
	fn.irProgInit.Do(func() {
		fn.irProg = compileFnProg(fn)
		if fn.irProg == nil {
			fn.irProg = irCompileFailed
		}
		atomic.StoreUint32(&fn.irProgOnce, 1)
	})
	if fn.irProg == irCompileFailed {
		return nil
	}
	return fn.irProg
}

func compileFnProg(fn *Fn) *IRProgram {
	// Check arity-level cache first to avoid recompiling per-instance.
	if len(fn.fnExpr.arities) == 1 {
		if cached, ok := irFnCache.Load(&fn.fnExpr.arities[0]); ok {
			prog := cached.(*IRProgram)
			if prog == irCompileFailed {
				return nil
			}
			// Arity cache has a prog, but it might have wrong captures
			// for this instance. Only reuse if no captures.
			if len(prog.captureSlots) == 0 {
				return prog
			}
		}
	}
	prog := irCompileFn(fn)
	if prog != nil {
		return prog
	}

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
	return prog
}
