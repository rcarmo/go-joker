package core_test

import (
	core "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"testing"
)

var multiplyResult coretypes.Number

func BenchmarkIntMultiplySmall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		multiplyResult = coretypes.INT_OPS.Multiply(coretypes.MakeInt(123), coretypes.MakeInt(456))
	}
}

// Invoice totals exercise the public numeric primitive over persistent input
// rows. Parsing is excluded and the checksum is validated before timing.
func BenchmarkInvoiceTotals(b *testing.B) {
	expr := compileBenchExpr(b, `(let [rows [[3 125] [7 249] [2 999] [12 49]]] (loop [i 0 total 0] (if (= i 100) total (recur (inc i) (+ total (reduce (fn [s row] (+ s (joker.core/multiply__ (first row) (second row)))) 0 rows))))))`)
	got := core.Eval(expr, nil)
	if n, ok := got.(coretypes.Int); !ok || n.I != 470400 {
		b.Fatalf("invalid invoice checksum: %v", got)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		core.Eval(expr, nil)
	}
}
