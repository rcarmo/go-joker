package core

// ir_call_dispatch.go — IR-aware function call dispatch for the tree-walker.
//
// When the tree-walker (CallExpr.Eval) calls a *Fn, this tries to dispatch
// through the IR executor (typed then boxed) before falling back to fn.Call.
// This eliminates environment frame allocation and enables irValue-based
// execution for compiled functions called from non-IR code paths.

func irDispatchFnCall(fn *Fn, args []Object) Object {
	// Only try IR dispatch for self-recursive fns (proven correct patterns)
	// and fns with native helpers. Other fns may have subtle correctness
	// differences between IR and tree-walker evaluation.
	if fnProg := irCompileFn(fn); fnProg != nil && (fnProg.hasSelf || fnProg.nativeHelper != nil) {
		var result Object
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
		stableArgs := make([]Object, len(args))
		copy(stableArgs, args)
		args = stableArgs
	}
	return fn.Call(args)
}
