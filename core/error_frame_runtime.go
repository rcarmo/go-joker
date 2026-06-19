package core

import (
	"fmt"

	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

func cloneGRT() *goroutineRT {
	grt := currentGRT()
	return &goroutineRT{
		Callstack:   grt.Callstack.Clone(),
		CurrentExpr: grt.CurrentExpr,
	}
}

func (rt *Runtime) NewError(msg string) *corert.EvalError {
	grt := cloneGRT()
	pos := coretypes.Position{}
	if grt.CurrentExpr != nil {
		pos = grt.CurrentExpr.(Expr).Pos()
	}
	return corert.NewEvalError(msg, pos, grt, LINTER_MODE)
}

func (rt *Runtime) NewArgTypeError(index int, obj coretypes.Object, expectedType string) *corert.EvalError {
	grt := currentGRT()
	name := "<unknown>"
	if grt.CurrentExpr != nil {
		if tr, ok := grt.CurrentExpr.(Traceable); ok {
			name = tr.Name()
		}
	}
	return rt.NewError(fmt.Sprintf("Arg[%d] of %s must have type %s, got %s", index, name, expectedType, obj.GetType().ToString(false)))
}

func (rt *Runtime) NewErrorWithPos(msg string, pos coretypes.Position) *corert.EvalError {
	return corert.NewEvalError(msg, pos, cloneGRT(), LINTER_MODE)
}

func (rt *Runtime) stacktrace() string {
	grt := currentGRT()
	return runtimeStacktrace(grt)
}

func runtimeStacktrace(grt *goroutineRT) string {
	var current Traceable
	if grt.CurrentExpr != nil {
		current, _ = grt.CurrentExpr.(Traceable)
	}
	return grt.Callstack.Stacktrace(current)
}

func (rt *Runtime) pushFrame() {
	grt := currentGRT()
	var tr Traceable
	if grt.CurrentExpr != nil {
		if t, ok := grt.CurrentExpr.(Traceable); ok {
			tr = t
		} else {
			tr = &CallExpr{}
		}
	} else {
		tr = &CallExpr{}
	}
	grt.Callstack.Push(tr)
}

func (rt *Runtime) popFrame() {
	grt := currentGRT()
	grt.Callstack.Pop()
}

func restoreCurrentExpr(expr any) {
	currentGRT().CurrentExpr = expr
}
