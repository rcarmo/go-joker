package core

import "testing"

func TestWasmOneHelperModule(t *testing.T) {
	expr := compileTestExpr(t, `(let [f (fn [x] (+ (* x x) 1))]
  (loop [i 0 acc 0]
    (if (= i 5)
      acc
      (recur (inc i) (+ acc (f i))))))`)
	letExpr := expr.(*LetExpr)
	fnObj := Eval(letExpr.values[0], nil)
	loop := letExpr.body[0].(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Fatal("expected loop IR")
	}
	slots := make([]Object, prog.numSlots)
	slots[0] = Int{I: 0}
	slots[1] = Int{I: 0}
	slots[2] = fnObj
	wp := wasmGetCachedWithOneHelper(prog, slots)
	if wp == nil {
		t.Fatal("expected one-helper WASM module")
	}
	requireInt(t, wasmExec(wp, slots), 35)
}
