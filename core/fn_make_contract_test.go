package core

import "testing"

func TestIRMakeFnCapturesCurrentSlots(t *testing.T) {
	expr := compileBenchExpr(t, `(loop [x 10]
  (fn [y] (+ x y)))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("expected loop returning fn to compile to IR")
	}
	if len(prog.fnExprs) != 1 {
		t.Fatalf("expected root executable envelope to retain one FnExpr, got %d", len(prog.fnExprs))
	}
	model := prog.neutralModel()
	if model == nil || model.ConstantsLen != len(prog.constants) {
		t.Fatalf("neutral model mismatch: model=%#v constants=%d", model, len(prog.constants))
	}
	fnObj := irExec(prog, []Object{MakeInt(10)})
	fn, ok := fnObj.(*Fn)
	if !ok {
		t.Fatalf("irMakeFn result = %T, want *Fn", fnObj)
	}
	if fn.env == nil || len(fn.env.bindings) == 0 || !fn.env.bindings[0].Equals(MakeInt(10)) {
		t.Fatalf("irMakeFn did not capture current slots: %#v", fn.env)
	}
	got := fn.Call([]Object{MakeInt(5)})
	if !got.Equals(MakeInt(15)) {
		t.Fatalf("captured fn result = %s, want 15", got.ToString(false))
	}
}
