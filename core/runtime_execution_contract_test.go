package core

import "testing"

func TestRuntimeExecutionAdapterExecutionFailureFlags(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	prog := &IRProgram{}
	if !adapter.CanExecuteIR(prog) || !adapter.CanExecuteTypedIR(prog) {
		t.Fatal("fresh program should be executable by boxed and typed IR")
	}
	adapter.MarkTypedExecutionFailed(prog)
	if !prog.typedFailed || !adapter.CanExecuteIR(prog) || adapter.CanExecuteTypedIR(prog) {
		t.Fatalf("typed failure should only disable typed IR: %#v", prog)
	}
	adapter.MarkBoxedExecutionFailed(prog)
	if !prog.execFailed || adapter.CanExecuteIR(prog) || adapter.CanExecuteTypedIR(prog) {
		t.Fatalf("boxed failure should disable all IR execution: %#v", prog)
	}
	if adapter.CanExecuteIR(nil) || adapter.CanExecuteTypedIR(nil) {
		t.Fatal("nil program must not be executable")
	}
}

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

func TestRuntimeExecutionAdapterApplyCaptureSlots(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	slots := []Object{NIL, NIL, NIL}
	if !adapter.ApplyCaptureSlots(slots, []int{2, 0}, []Object{MakeInt(20), MakeInt(10)}) {
		t.Fatal("ApplyCaptureSlots returned false for valid captures")
	}
	if !slots[0].Equals(MakeInt(10)) || !slots[2].Equals(MakeInt(20)) {
		t.Fatalf("capture slots = %#v", slots)
	}
	if adapter.ApplyCaptureSlots(slots, []int{3}, []Object{MakeInt(1)}) {
		t.Fatal("ApplyCaptureSlots accepted out-of-range slot")
	}
	if adapter.ApplyCaptureSlots(slots, []int{1, 2}, []Object{MakeInt(1)}) {
		t.Fatal("ApplyCaptureSlots accepted mismatched metadata")
	}
}

func TestRuntimeExecutionAdapterApplyTypedCaptureSlots(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	slots := make([]irValue, 2)
	if !adapter.ApplyTypedCaptureSlots(slots, []int{1}, []Object{MakeInt(42)}) {
		t.Fatal("ApplyTypedCaptureSlots returned false for valid captures")
	}
	if slots[1].tag != irValInt || slots[1].i != 42 {
		t.Fatalf("typed capture slot = %#v", slots[1])
	}
	if adapter.ApplyTypedCaptureSlots(slots, []int{-1}, []Object{MakeInt(1)}) {
		t.Fatal("ApplyTypedCaptureSlots accepted out-of-range slot")
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
