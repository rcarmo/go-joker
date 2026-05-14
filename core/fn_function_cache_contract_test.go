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
