package core

import "fmt"

// RuntimeExecutionAdapter is the narrow root-owned runtime surface that future
// extracted IR executors should target instead of reaching through all of core.
// It is intentionally small and grows only when contract tests justify a new
// operation.
type RuntimeExecutionAdapter struct{}

var runtimeExec RuntimeExecutionAdapter

func (RuntimeExecutionAdapter) Errorf(format string, args ...any) Error {
	return RT.NewError(fmt.Sprintf(format, args...))
}

func (r RuntimeExecutionAdapter) Throw(obj Object) {
	panic(r.Errorf("%s", obj.ToString(false)))
}

func (RuntimeExecutionAdapter) ApplyCaptureSlots(slots []Object, idxs []int, values []Object) bool {
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

func (RuntimeExecutionAdapter) ApplyTypedCaptureSlots(slots []irValue, idxs []int, values []Object) bool {
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

func (r RuntimeExecutionAdapter) PrepareCallSlots(prog *IRProgram, args []Object, env *LocalEnv) []Object {
	if prog == nil || len(prog.captureKeys) == 0 {
		return args
	}
	full := make([]Object, prog.numSlots)
	copy(full, args)
	r.InstallEnvCaptures(prog, full, env)
	return full
}

func (RuntimeExecutionAdapter) InstallEnvCaptures(prog *IRProgram, slots []Object, env *LocalEnv) {
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

func (RuntimeExecutionAdapter) MakeFn(fnExpr *FnExpr, slots []Object) Object {
	fnEnv := &LocalEnv{bindings: make([]Object, len(slots))}
	copy(fnEnv.bindings, slots)
	return &Fn{fnExpr: fnExpr, env: fnEnv}
}

func (RuntimeExecutionAdapter) CallObject(fnObj Object, args []Object) (Object, bool) {
	callable, ok := fnObj.(Callable)
	if !ok {
		return nil, false
	}
	return callable.Call(args), true
}

func (adapter RuntimeExecutionAdapter) CallObjectWithSyntheticCallExpr(fnObj Object, args []Object) (Object, bool) {
	grt := currentGRT()
	prevExpr := grt.currentExpr
	grt.currentExpr = &CallExpr{}
	result, ok := adapter.CallObject(fnObj, args)
	grt.currentExpr = prevExpr
	return result, ok
}

func (RuntimeExecutionAdapter) PersistentResult(result Object) Object {
	switch v := result.(type) {
	case *TransientVector:
		return v.ToPersistent()
	case *TransientMap:
		return v.ToPersistent()
	case *TransientString:
		return v.ToPersistent()
	default:
		return result
	}
}

func (RuntimeExecutionAdapter) Get(coll Object, key Object, def Object) Object {
	if g, ok := coll.(Gettable); ok {
		if ok, v := g.Get(key); ok {
			return v
		}
	}
	return def
}

func (RuntimeExecutionAdapter) Assoc(coll Object, key Object, val Object) (Object, bool) {
	switch c := coll.(type) {
	case *TransientVector:
		return c.AssocInPlace(key, val), true
	case *TransientMap:
		return c.AssocInPlace(key, val), true
	case Associative:
		return c.Assoc(key, val), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) Nth(coll Object, idx int) (Object, bool) {
	switch c := coll.(type) {
	case *ArrayVector:
		if idx >= 0 && idx < len(c.arr) {
			return c.arr[idx], true
		}
	case *TransientVector:
		if idx >= 0 && idx < len(c.arr) {
			return c.arr[idx], true
		}
	case String:
		return stringNthFast(c.S, idx), true
	case Indexed:
		return c.Nth(idx), true
	}
	return nil, false
}

func (RuntimeExecutionAdapter) Conj(coll Object, val Object) (Object, bool) {
	switch c := coll.(type) {
	case *TransientVector:
		return c.ConjInPlace(val), true
	case Conjable:
		return c.Conj(val), true
	default:
		return nil, false
	}
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

func (RuntimeExecutionAdapter) ProgramConstant(prog *IRProgram, idx int) (Object, bool) {
	if prog == nil || idx < 0 || idx >= len(prog.constants) {
		return nil, false
	}
	return prog.constants[idx], true
}

func (RuntimeExecutionAdapter) ProgramConstants(prog *IRProgram) []Object {
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

func (RuntimeExecutionAdapter) ProgramAnalysis(prog *IRProgram) IRAnalysis {
	return AnalyzeIRProgram(prog)
}

func (adapter RuntimeExecutionAdapter) ApplyProgramCaptureSlots(prog *IRProgram, slots []Object) bool {
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

func (RuntimeExecutionAdapter) ProgramCaptureSlots(prog *IRProgram) ([]int, []Object) {
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
