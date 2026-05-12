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

func (RuntimeExecutionAdapter) MakeFn(fnExpr *FnExpr, slots []Object) Object {
	fnEnv := &LocalEnv{bindings: make([]Object, len(slots))}
	copy(fnEnv.bindings, slots)
	return &Fn{fnExpr: fnExpr, env: fnEnv}
}
