package core

import "testing"

func TestRuntimeExecutionAdapterPrepareCallSlotsInstallsCaptures(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	prog := &IRProgram{
		numSlots:        3,
		captureKeys:     []bindingKey{{index: 0}},
		captureSlotIdxs: []int{2},
	}
	env := &LocalEnv{bindings: []Object{MakeString("captured")}}
	args := []Object{MakeInt(1)}
	full := adapter.PrepareCallSlots(prog, args, env)
	if len(full) != 3 || full[0] != args[0] || full[2].(String).S != "captured" {
		t.Fatalf("prepared call slots mismatch: %#v", full)
	}
	if got := adapter.PrepareCallSlots(&IRProgram{}, args, env); len(got) != 1 || got[0] != args[0] {
		t.Fatalf("capture-free call should reuse args: %#v", got)
	}
}

func TestRuntimeExecutionAdapterInstallsTypedEnvCaptures(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	prog := &IRProgram{
		numSlots:        2,
		captureKeys:     []bindingKey{{index: 0}},
		captureSlotIdxs: []int{1},
	}
	env := &LocalEnv{bindings: []Object{MakeInt(42)}}
	slots := make([]irValue, 2)
	adapter.InstallTypedEnvCaptures(prog, slots, env)
	if slots[1].tag != irValInt || slots[1].i != 42 {
		t.Fatalf("typed env capture slot = %#v", slots[1])
	}
}

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

func TestRuntimeExecutionAdapterNativeHelperState(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	prog := &IRProgram{}
	if adapter.HasNativeHelper(prog) || adapter.NativeHelperChecked(prog) {
		t.Fatal("fresh program should not have a checked native helper")
	}
	if helper, ok := adapter.NativeHelper(prog); helper != nil || ok {
		t.Fatalf("NativeHelper = %v, %v; want nil, false", helper, ok)
	}
	helper := nativeF64Fn(func(args []float64) float64 { return args[0] + 1 })
	adapter.InstallNativeHelper(prog, helper)
	got, ok := adapter.NativeHelper(prog)
	if !ok || got([]float64{2}) != 3 {
		t.Fatal("native helper was not installed through adapter")
	}
	if !adapter.HasNativeHelper(prog) || !adapter.NativeHelperChecked(prog) {
		t.Fatal("adapter did not expose installed native helper state")
	}
}

func TestRuntimeExecutionAdapterMemNthFallbackState(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	prog := &IRProgram{}
	if !adapter.CanTryMemNth(prog) {
		t.Fatal("fresh program should allow mem-nth attempt")
	}
	adapter.MarkMemNthFailed(prog)
	if adapter.CanTryMemNth(prog) || !prog.memNthFailed {
		t.Fatal("MarkMemNthFailed did not disable mem-nth attempts")
	}
	if adapter.CanTryMemNth(nil) {
		t.Fatal("nil program must not allow mem-nth attempts")
	}
}
