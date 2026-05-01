package core

import "testing"

func TestIRTransientStringBuilder(t *testing.T) {
	t.Setenv("JOKER_IR_STRING_BUILDER", "force")
	requireString(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      s
      (recur (inc i) (str s (nth dna i))))))`), "ACGT")
}

func TestIRTransientStringPrependAuto(t *testing.T) {
	requireString(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      s
      (recur (inc i) (str (nth dna i) s)))))`), "TGCA")
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

func BenchmarkIRStringPrependBuilderLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(let [dna "GGTATTTTAATTTATAGT"]
  (loop [i 0 s ""]
    (if (= i 128)
      (count s)
      (recur (inc i) (str (nth dna (rem i 18)) s)))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}
