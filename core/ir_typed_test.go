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

func BenchmarkIRTypedStringLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(let [dna "GGTATTTTAATTTATAGT"]
  (loop [i 0 s ""]
    (if (= i 128)
      (count s)
      (recur (inc i) (str s (nth dna (rem i 18)))))))`)
	letExpr := expr.(*LetExpr)
	prog := irCompile(letExpr.body[0].(*LoopExpr))
	init := []Object{Int{I: 0}, String{S: ""}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = irExecTyped(prog, init)
	}
}
