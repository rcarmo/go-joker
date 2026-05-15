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

func TestIREqSupportsStringsAndChars(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [f (fn [c]
                                  (if (= c "A") 1
                                  (if (= c "T") 2 3)))]
  (loop [i 0 acc 0]
    (if (= i 3)
      acc
      (recur (inc i) (+ acc (f (str (nth "ATA" i))))))))`), 4)
}

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
