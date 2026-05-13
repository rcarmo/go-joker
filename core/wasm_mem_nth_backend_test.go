package core

import "testing"

func TestWasmMemNthSimple(t *testing.T) {
	// Sum vector elements via nth in WASM with linear memory
	code := `(let [v [10.0 20.0 30.0]]
	  (loop [j 0 s 0.0]
	    (if (= j 3) s
	      (recur (+ j 1) (+ s (nth v j))))))`
	expr := compileTestExpr(t, code)
	letExpr := expr.(*LetExpr)
	loop := letExpr.body[0].(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Fatal("not compiled")
	}
	t.Logf("eligible=%v", wasmMemNthEligible(prog, nil))

	// Use full eval to test
	r := Eval(expr, nil)
	t.Logf("eval result=%s", r.ToString(false))
	requireDouble(t, r, 60.0)
}

func TestWasmMemNthWithHelper(t *testing.T) {
	clbgInit()
	// Spectral-norm pattern: A helper + vector nth
	code := `(let [A (fn [i j] (/ 1.0 (+ (/ (* (+ i j) (+ (+ i j) 1)) 2) (+ i 1))))
	              v [1.0 1.0 1.0 1.0 1.0]]
	  (loop [j 0 s 0.0]
	    (if (= j 5) s
	      (recur (+ j 1) (+ s (* (A 0 j) (nth v j)))))))`
	r := evalTestScript(t, code)
	t.Logf("result=%s", r.ToString(false))
}
