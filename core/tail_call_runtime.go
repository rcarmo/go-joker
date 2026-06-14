package core

import (
	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

// ---- tail_call.go ----
// tco.go — generic tail-call optimization via trampoline.
//
// When a function body's tail expression is a call to the same function,
// it returns a TailCall marker instead of actually recursing. Fn.Call
// detects this and loops with the new args, eliminating stack growth.
//
// This benefits any self-recursive function where the self-call is in
// tail position (e.g. accumulators, state machines, list traversals).
// It does NOT help tree-recursive patterns like naive fib where the
// recursive calls are not in tail position.

// TailCall is a marker returned by evalTailExpr when a self-call in
// tail position is detected. It is NOT a valid Joker coretypes.Object — it is
// only used internally between evalLoop and Fn.Call.
type TailCall struct {
	fn   *Fn
	args []coretypes.Object
}

// coretypes.Object interface stubs — TailCall should never escape to user code.
func (tc *TailCall) ToString(escape bool) string                     { return "#<tail-call>" }
func (tc *TailCall) Equals(other interface{}) bool                   { return false }
func (tc *TailCall) GetInfo() *coretypes.ObjectInfo                  { return nil }
func (tc *TailCall) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return tc }
func (tc *TailCall) GetType() *coretypes.Type                        { return TYPE.Fn }
func (tc *TailCall) Hash() uint32                                    { return 0 }

// activeFn tracks the currently executing Fn for TCO detection.
// This is stored on the Runtime (single-threaded evaluator).
var activeFn *Fn

// evalBodyTCO evaluates a body and, for the last expression, checks
// if it's a self-call in tail position. If so, returns a *TailCall
// instead of actually calling.
func evalBodyTCO(body []Expr, env *LocalEnv, self *Fn) coretypes.Object {
	if len(body) == 0 {
		return NIL
	}
	// Evaluate all but the last expression normally
	for i := 0; i < len(body)-1; i++ {
		Eval(body[i], env)
	}
	// For the last expression, check for tail self-call
	last := body[len(body)-1]
	return evalTailExpr(last, env, self)
}

// evalLoopTCO is like evalLoop but with TCO awareness.
func evalLoopTCO(body []Expr, env *LocalEnv, self *Fn) coretypes.Object {
	var res coretypes.Object = NIL
loop:
	for _, expr := range body {
		res = Eval(expr, env)
	}
	switch res := res.(type) {
	default:
		return res
	case coretypes.RecurBindings:
		env.bindings = res
		goto loop
	}
}

// evalTailExpr evaluates an expression in tail position with self-call detection.
func evalTailExpr(expr Expr, env *LocalEnv, self *Fn) coretypes.Object {
	switch e := expr.(type) {
	case *IfExpr:
		if corert.ToBool(Eval(e.cond, env)) {
			return evalTailExpr(e.positive, env, self)
		}
		return evalTailExpr(e.negative, env, self)

	case *CallExpr:
		// Check if this is a self-call
		callable := Eval(e.callable, env)
		if fn, ok := callable.(*Fn); ok && fn == self {
			// This is a tail self-call — return TailCall marker
			args := evalSeq(e.args, env)
			return &TailCall{fn: fn, args: args}
		}
		// Not a self-call — evaluate normally
		switch c := callable.(type) {
		case coretypes.Callable:
			args := evalSeq(e.args, env)
			return c.Call(args)
		default:
			panic(RT.NewErrorWithPos(callable.ToString(false)+" is not a Fn", e.callable.Pos()))
		}

	case *DoExpr:
		return evalBodyTCO(e.body, env, self)

	case *LetExpr:
		childEnv := LocalEnv{bindings: make([]coretypes.Object, 0, len(e.names)), parent: env}
		if env != nil {
			childEnv.frame = env.frame + 1
		}
		for _, bindingExpr := range e.values {
			childEnv.bindings = append(childEnv.bindings, Eval(bindingExpr, &childEnv))
		}
		return evalBodyTCO(e.body, &childEnv, self)

	default:
		// Not a recognized tail form — evaluate normally
		return Eval(expr, env)
	}
}

// ---- tco_rewrite.go ----
// tco_rewrite.go — parse-time rewriting of tail-self-calls to recur.
//
// When a named fn (from letfn or named fn) has a tail-position call
// to itself, rewrite the fn body as a loop/recur. This eliminates
// the runtime trampoline overhead entirely.
//
// Before: (fn self [x] (if (= x 0) 1 (self (dec x))))
// After:  (fn self [x] (loop [x x] (if (= x 0) 1 (recur (dec x)))))
//
// The rewrite wraps the body in a LoopExpr with the fn args as bindings,
// and replaces tail self-calls with RecurExpr.

// rewriteTailCallsToRecur checks if a FnExpr with a self-binding
// has tail-position self-calls, and if so, rewrites them to recur.
func rewriteTailCallsToRecur(fnExpr *FnExpr, selfBinding *Binding) {
	if selfBinding == nil || fnExpr.self.NameKey() == nil {
		return
	}
	for i := range fnExpr.arities {
		arity := &fnExpr.arities[i]
		if len(arity.body) == 0 {
			continue
		}
		lastExpr := arity.body[len(arity.body)-1]
		has := hasTailSelfCall(lastExpr, selfBinding)
		if has {
			newBody := make([]Expr, len(arity.body))
			copy(newBody, arity.body)
			newBody[len(newBody)-1] = rewriteTailExpr(newBody[len(newBody)-1], selfBinding)
			arity.body = newBody
			fnExpr.tailRewritten = true
		}
	}
}

// hasTailSelfCall checks if an expression in tail position calls selfBinding.
func hasTailSelfCall(expr Expr, self *Binding) bool {
	switch e := expr.(type) {
	case *CallExpr:
		if bind, ok := e.callable.(*BindingExpr); ok {
			// The self-call may be through the letfn binding or the fn's own self binding.
			// Match by name since they may have different frame/index.
			if bind.binding.name.NameKey() != nil && self.name.NameKey() != nil &&
				*bind.binding.name.NameKey() == *self.name.NameKey() {
				return true
			}
		}
		return false
	case *IfExpr:
		return hasTailSelfCall(e.positive, self) || hasTailSelfCall(e.negative, self)
	case *DoExpr:
		if len(e.body) == 0 {
			return false
		}
		return hasTailSelfCall(e.body[len(e.body)-1], self)
	case *LetExpr:
		if len(e.body) == 0 {
			return false
		}
		return hasTailSelfCall(e.body[len(e.body)-1], self)
	default:
		return false
	}
}

// rewriteTailExpr replaces tail-position self-calls with RecurExpr.
func rewriteTailExpr(expr Expr, self *Binding) Expr {
	switch e := expr.(type) {
	case *CallExpr:
		if bind, ok := e.callable.(*BindingExpr); ok {
			if bind.binding.name.NameKey() != nil && self.name.NameKey() != nil &&
				*bind.binding.name.NameKey() == *self.name.NameKey() {
				return &RecurExpr{
					Position: e.Position,
					args:     e.args,
				}
			}
		}
		return e
	case *IfExpr:
		return &IfExpr{
			Position: e.Position,
			cond:     e.cond,
			positive: rewriteTailExpr(e.positive, self),
			negative: rewriteTailExpr(e.negative, self),
		}
	case *DoExpr:
		if len(e.body) == 0 {
			return e
		}
		newBody := make([]Expr, len(e.body))
		copy(newBody, e.body)
		newBody[len(newBody)-1] = rewriteTailExpr(newBody[len(newBody)-1], self)
		return &DoExpr{
			Position: e.Position,
			body:     newBody,
		}
	case *LetExpr:
		if len(e.body) == 0 {
			return e
		}
		newBody := make([]Expr, len(e.body))
		copy(newBody, e.body)
		newBody[len(newBody)-1] = rewriteTailExpr(newBody[len(newBody)-1], self)
		return &LetExpr{
			Position: e.Position,
			names:    e.names,
			values:   e.values,
			body:     newBody,
		}
	default:
		return expr
	}
}
