package core

import "testing"

func TestIRConstantCountFoldsStringBinding(t *testing.T) {
	expr := compileTestExpr(t, `(let [s "abcdef"]
  (loop [i 0 acc 0]
    (if (= i 3)
      acc
      (recur (inc i) (+ acc (count s))))))`)
	letExpr := expr.(*LetExpr)
	prog := irCompile(letExpr.body[0].(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	for pc := 0; pc < len(prog.code); pc++ {
		if prog.code[pc] == irCount {
			t.Fatal("expected count to be folded")
		}
	}
	requireInt(t, Eval(expr, nil), 18)
}
