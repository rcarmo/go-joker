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

func BenchmarkIRTypedGenericStringNthLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(loop [i 0 s "GGTATTTTAATTTATAGT" acc ""]
  (if (= i 128)
    (count acc)
    (recur (inc i) s (str acc (nth s (rem i 18))))))`)
	prog := irCompile(expr.(*LoopExpr))
	init := []Object{Int{I: 0}, String{S: "GGTATTTTAATTTATAGT"}, String{S: ""}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = irExecTyped(prog, init)
	}
}
