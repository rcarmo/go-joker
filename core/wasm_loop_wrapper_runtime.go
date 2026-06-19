package core

import coretypes "github.com/rcarmo/go-joker/core/types"

func buildWasmLoopWrapper(fn *Fn, arity FnArityExpr, loop *LoopExpr, loopProg *IRProgram) coretypes.Object {
	le := (*LetExpr)(loop)
	nLoopBinds := len(le.names)
	nSlots := loopProg.numSlots
	capKeys := loopProg.captureKeys

	wp := wasmCompile(loopProg)
	if wp == nil {
		return nil
	}

	initVals := make([]coretypes.Object, nLoopBinds)
	for i, v := range le.values {
		lit, ok := v.(*LiteralExpr)
		if !ok {
			return nil
		}
		switch lit.obj.(type) {
		case coretypes.Int, coretypes.Double:
			initVals[i] = lit.obj
		default:
			return nil
		}
	}

	fnParamFrame := -1
	for _, ck := range capKeys {
		if ck.index < len(arity.args) {
			if fnParamFrame < 0 {
				fnParamFrame = ck.frame
			} else if fnParamFrame != ck.frame {
				break
			}
		}
	}
	type capInfo struct {
		isDynamic bool
		argIdx    int
		constVal  coretypes.Object
	}
	caps := make([]capInfo, len(capKeys))
	for ci, ck := range capKeys {
		if ck.frame == fnParamFrame && ck.index < len(arity.args) {
			caps[ci] = capInfo{isDynamic: true, argIdx: ck.index}
			continue
		}
		resolved := false
		for e := fn.env; e != nil; e = e.parent {
			if ck.index < len(e.bindings) {
				switch e.bindings[ck.index].(type) {
				case coretypes.Int, coretypes.Double:
					caps[ci] = capInfo{constVal: e.bindings[ck.index]}
					resolved = true
				}
				if resolved {
					break
				}
			}
		}
		if !resolved {
			return nil
		}
	}

	return Proc{
		Fn: func(args []coretypes.Object) coretypes.Object {
			if len(args) != len(arity.args) {
				PanicArityMinMax(len(args), len(arity.args), len(arity.args))
			}
			var buf [16]coretypes.Object
			var slots []coretypes.Object
			if nSlots <= len(buf) {
				slots = buf[:nSlots]
			} else {
				slots = make([]coretypes.Object, nSlots)
			}
			copy(slots[:nLoopBinds], initVals)
			for ci, cap := range caps {
				if cap.isDynamic {
					slots[nLoopBinds+ci] = args[cap.argIdx]
				} else {
					slots[nLoopBinds+ci] = cap.constVal
				}
			}
			result := wasmExec(wp, slots)
			if result == nil {
				panic(RT.NewError("jit/compile-wasm: WASM execution failed"))
			}
			return result
		},
		Name: "jit-wasm-loop",
	}
}
