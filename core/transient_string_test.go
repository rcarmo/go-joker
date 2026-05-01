package core

import "testing"

func TestIRTransientStringBuilder(t *testing.T) {
	t.Setenv("JOKER_IR_STRING_BUILDER", "1")
	requireString(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      s
      (recur (inc i) (str s (nth dna i))))))`), "ACGT")
}

func BenchmarkIRStringBuilderLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(let [dna "GGTATTTTAATTTATAGT"]
  (loop [i 0 s ""]
    (if (= i 256)
      (count s)
      (recur (inc i) (str s (nth dna (rem i 18)))))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}
