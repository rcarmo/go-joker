package core

import "fmt"

// RuntimeExecutionAdapter is the narrow root-owned runtime surface that future
// extracted IR executors should target instead of reaching through all of core.
// It is intentionally small and grows only when contract tests justify a new
// operation.
type RuntimeExecutionAdapter struct{}

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

func (RuntimeExecutionAdapter) MakeFn(fnExpr *FnExpr, slots []Object) Object {
	fnEnv := &LocalEnv{bindings: make([]Object, len(slots))}
	copy(fnEnv.bindings, slots)
	return &Fn{fnExpr: fnExpr, env: fnEnv}
}
