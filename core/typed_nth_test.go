package core

import "testing"

func TestIRTypedGenericStringNth(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 s "ACGT" acc ""]
  (if (= i 4)
    acc
    (recur (inc i) s (str acc (nth s i)))))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	got := irExecTyped(prog, []Object{Int{I: 0}, String{S: "ACGT"}, String{S: ""}})
	requireString(t, got, "ACGT")
}
