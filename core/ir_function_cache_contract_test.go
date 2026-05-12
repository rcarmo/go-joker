package core

import "testing"

func TestIRFunctionCacheUsesStableArityKeys(t *testing.T) {
	expr := compileBenchExpr(t, `(fn [x] (+ x 1))`)
	fn := Eval(expr, nil).(*Fn)
	first := irCompileFn(fn)
	if first == nil {
		t.Fatal("first irCompileFn returned nil")
	}
	second := irCompileFn(fn)
	if second == nil {
		t.Fatal("second irCompileFn returned nil")
	}
	if first != second {
		t.Fatalf("irCompileFn returned different programs for same fn: %p != %p", first, second)
	}
}

func TestIRFunctionCacheUsesStableVariadicKey(t *testing.T) {
	expr := compileBenchExpr(t, `(fn [& xs] (count xs))`)
	fn := Eval(expr, nil).(*Fn)
	first := irCompileFn(fn)
	if first == nil {
		t.Fatal("first variadic irCompileFn returned nil")
	}
	second := irCompileFn(fn)
	if second == nil {
		t.Fatal("second variadic irCompileFn returned nil")
	}
	if first != second {
		t.Fatalf("variadic irCompileFn returned different programs for same fn: %p != %p", first, second)
	}
}
