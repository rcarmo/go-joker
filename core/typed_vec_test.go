package core

import "testing"

func TestIRTypedIntVector(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED_VEC", "1")
	requireInt(t, evalTestScript(t, `(loop [i 0 v []]
  (if (= i 5)
    (+ (nth v 0) (nth v 4))
    (recur (inc i) (conj v i))))`), 4)
}

func BenchmarkIRTypedIntVectorLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(loop [i 0 v []]
  (if (= i 1000)
    (+ (nth v 0) (nth v 999))
    (recur (inc i) (conj v i))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}
