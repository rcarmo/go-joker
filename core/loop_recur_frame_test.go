package core

import "testing"

func TestIRGuessLoopFramePrefersRecurBindings(t *testing.T) {
	expr := compileTestExpr(t, `(let [limit 4]
  (loop [i 0 acc 0]
    (if (< i limit)
      (let [x (loop [j 0 s 0]
                (if (= j i) s (recur (inc j) (+ s j))))]
        (recur (inc i) (+ acc x)))
      acc)))`)
	outer := expr.(*LetExpr).body[0].(*LoopExpr)
	if prog := irCompile(outer); prog == nil {
		t.Fatal("expected outer loop with nested loop/captures to compile")
	}
	requireInt(t, Eval(expr, nil), 4)
}
