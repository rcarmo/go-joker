package core

import "testing"

func BenchmarkIRStringAppendCharLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(let [dna "GGTATTTTAATTTATAGT"]
  (loop [i 0 s ""]
    (if (= i 64)
      (count s)
      (recur (inc i) (str s (nth dna (rem i 18)))))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkIRCharCompareLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(let [dna "GGTATTTTAATTTATAGT"]
  (loop [i 0 acc 0]
    (if (= i 256)
      acc
      (let [c (nth dna (rem i 18))]
        (recur (inc i) (if (= c \T) (inc acc) acc))))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}
