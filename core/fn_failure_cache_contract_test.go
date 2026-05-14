package core

import (
	"sync/atomic"
	"testing"
)

func TestIRCompileFailureIsCachedOnFn(t *testing.T) {
	fn := evalTestScript(t, `(fn [x] (println x))`).(*Fn)
	if prog := irGetFnProg(fn); prog != nil {
		t.Fatalf("string-building fn unexpectedly compiled to IR: %#v", prog)
	}
	if atomic.LoadUint32(&fn.irProgOnce) != 1 {
		t.Fatal("IR compile failure should mark fn cache as initialized")
	}
	if fn.irProg != irCompileFailed {
		t.Fatalf("IR compile failure should cache irCompileFailed sentinel, got %#v", fn.irProg)
	}
}

func TestNativeHelperEligibilityContract(t *testing.T) {
	pure := evalTestScript(t, `(fn [x y] (+ (* x x) y))`).(*Fn)
	pureProg := irGetFnProg(pure)
	nativeHelper, ok := runtimeExec.NativeHelper(pureProg)
	if pureProg == nil || !ok || !runtimeExec.NativeHelperChecked(pureProg) {
		t.Fatalf("pure numeric helper should compile native helper: %#v", pureProg)
	}
	if got := nativeHelper([]float64{3, 4}); got != 13 {
		t.Fatalf("native helper result = %f, want 13", got)
	}

	impure := evalTestScript(t, `(fn [x] [x])`).(*Fn)
	impureProg := irGetFnProg(impure)
	if impureProg == nil {
		t.Fatal("vector-building fn should still compile to boxed IR")
	}
	if runtimeExec.HasNativeHelper(impureProg) {
		t.Fatal("collection-building fn must not get a numeric native helper")
	}
	if !runtimeExec.NativeHelperChecked(impureProg) {
		t.Fatal("native helper eligibility should be checked and cached")
	}
}
