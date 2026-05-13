package core

import "testing"

func TestIRInlineCollectionHelper(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [pick (fn [v i] (+ (nth v i) 1))
                                      xs [1 2 3 4]]
  (loop [i 0 acc 0]
    (if (= i 4)
      acc
      (recur (inc i) (+ acc (pick xs i))))))`), 14)
}

func BenchmarkIRInlineCollectionHelperLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(let [pick (fn [v i] (+ (nth v i) 1))
                                  xs [1 2 3 4 5 6 7 8]]
  (loop [i 0 acc 0]
    (if (= i 1000)
      acc
      (recur (inc i) (+ acc (pick xs (rem i 8)))))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}
