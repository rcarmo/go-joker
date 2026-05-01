package core

import "testing"

func TestIRTypedStringIntMap(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED_MAP", "1")
	requireInt(t, evalTestScript(t, `(loop [i 0 m {}]
  (if (= i 4)
    (get m "aa" 0)
    (recur (inc i) (assoc m "aa" (inc (get m "aa" 0))))))`), 4)
}

func BenchmarkIRTypedStringIntMapLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(let [ks ["aa" "bb" "cc" "dd"]]
  (loop [i 0 m {}]
    (if (= i 1000)
      (+ (get m "aa" 0) (get m "dd" 0))
      (let [k (nth ks (rem i 4))]
        (recur (inc i) (assoc m k (inc (get m k 0))))))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}
