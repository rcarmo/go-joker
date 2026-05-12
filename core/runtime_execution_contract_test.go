package core

import "testing"

func TestRuntimeExecutionAdapterMakeFnCapturesSlots(t *testing.T) {
	expr := compileBenchExpr(t, `(fn [y] y)`)
	fnExpr := expr.(*FnExpr)
	adapter := RuntimeExecutionAdapter{}
	fnObj := adapter.MakeFn(fnExpr, []Object{MakeInt(10)})
	fn, ok := fnObj.(*Fn)
	if !ok {
		t.Fatalf("MakeFn returned %T, want *Fn", fnObj)
	}
	if fn.env == nil || len(fn.env.bindings) != 1 || !fn.env.bindings[0].Equals(MakeInt(10)) {
		t.Fatalf("MakeFn did not capture slots: %#v", fn.env)
	}
}

func TestRuntimeExecutionAdapterErrorf(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	err := adapter.Errorf("contract %d", 42)
	msg, ok := err.Message().(String)
	if err == nil || !ok || msg.S != "contract 42" {
		t.Fatalf("Errorf = %#v", err)
	}
}

func TestRuntimeExecutionAdapterThrow(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	defer func() {
		r := recover()
		err, ok := r.(Error)
		if !ok {
			t.Fatalf("Throw panic = %T, want core Error", r)
		}
		msg := err.Message().(String)
		if msg.S != "boom" {
			t.Fatalf("Throw message = %q, want boom", msg.S)
		}
	}()
	adapter.Throw(MakeString("boom"))
}
