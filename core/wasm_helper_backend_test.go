package core

import (
	"testing"
)

func TestWasmOneHelperModule(t *testing.T) {
	// Use a helper with a string op so auto-inlining won't absorb it
	// (auto inlines text helpers but not ones with both text and non-text mixed patterns)
	t.Setenv("JOKER_IR_INLINE", "off")
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

func TestWasmOneHelperFloatRequiresForce(t *testing.T) {
	expr := compileTestExpr(t, `(let [f (fn [x] (* x 2.0))]
  (loop [i 0 acc 0.0]
    (if (= i 2)
      acc
      (recur (inc i) (+ acc (f 1.5))))))`)
	letExpr := expr.(*LetExpr)
	fnObj := Eval(letExpr.values[0], nil)
	loop := letExpr.body[0].(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Fatal("expected loop IR")
	}
	slots := make([]Object, prog.numSlots)
	slots[0] = Int{I: 0}
	slots[1] = Double{D: 0}
	slots[2] = fnObj
	if wp := wasmGetCachedWithOneHelper(prog, slots); wp != nil {
		t.Fatal("float helper should be gated off in auto mode")
	}
}
