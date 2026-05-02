package core

import "sync/atomic"

// irGetFnProg returns the cached IR program for a Fn, compiling on first access.
// Uses atomic flag for lock-free single-check.
func irGetFnProg(fn *Fn) *IRProgram {
	if atomic.LoadUint32(&fn.irProgOnce) == 1 {
		if fn.irProg == irCompileFailed {
			return nil
		}
		return fn.irProg
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
					if loopProg != nil && loopProg.nativeHelper != nil {
						// Build a wrapper that maps fn args to loop init slots.
						// The loop has: [loop bindings..., captures...]
						// Captures reference the fn's parameter frame.
						le := (*LetExpr)(loop)
						nLoopBinds := len(le.names)
						nSlots := loopProg.numSlots
						capKeys := loopProg.captureKeys
						// Pre-compute loop init values (must be literals: 0, 0.0, etc.)
						initVals := make([]float64, nLoopBinds)
						canWrap := true
						for i, v := range le.values {
							switch lit := v.(type) {
							case *LiteralExpr:
								switch lv := lit.obj.(type) {
								case Int:
									initVals[i] = float64(lv.I)
								case Double:
									initVals[i] = lv.D
								default:
									canWrap = false
								}
							default:
								canWrap = false
							}
						}
						if canWrap {
							// Build capture-to-fnarg mapping
							fnFrame := guessLoopFrame(arity.body)
							capArgIdx := make([]int, len(capKeys))
							for ci, ck := range capKeys {
								if ck.frame == fnFrame {
									capArgIdx[ci] = ck.index
								} else {
									canWrap = false
									break
								}
							}
							if canWrap {
								loopNative := loopProg.nativeHelper
								wrapper := func(fnArgs []float64) float64 {
									var buf [16]float64
									var loopArgs []float64
									if nSlots <= len(buf) {
										loopArgs = buf[:nSlots]
									} else {
										loopArgs = make([]float64, nSlots)
									}
									copy(loopArgs[:nLoopBinds], initVals)
									for ci, argIdx := range capArgIdx {
										loopArgs[nLoopBinds+ci] = fnArgs[argIdx]
									}
									return loopNative(loopArgs)
								}
								prog = &IRProgram{
									numSlots:      len(arity.args),
									nativeHelper:  wrapper,
									nativeChecked: true,
								}
							}
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
