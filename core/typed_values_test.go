package core

import "testing"

func TestIRTypedStringLoop(t *testing.T) {
	expr := compileTestExpr(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      (count s)
      (recur (inc i) (str s (nth dna i))))))`)
	letExpr := expr.(*LetExpr)
	prog := irCompile(letExpr.body[0].(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	got := irExecTyped(prog, []Object{Int{I: 0}, String{S: ""}})
	requireInt(t, got, 4)
}

func TestIRTypedStringBuilderSlot(t *testing.T) {
	expr := compileTestExpr(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      s
      (recur (inc i) (str s (nth dna i))))))`)
	letExpr := expr.(*LetExpr)
	prog := irCompile(letExpr.body[0].(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	got := irExecTyped(prog, []Object{Int{I: 0}, String{S: ""}})
	requireString(t, got, "ACGT")
}
