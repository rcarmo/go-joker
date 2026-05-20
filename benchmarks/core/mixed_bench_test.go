package core_test

import (
	"testing"

	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
	"github.com/tetratelabs/wazero"
)

// From core/inline_rewrites_test.go
func BenchmarkIRInlineSmallHelperLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(let [f (fn [x] (+ x 1))]
  (loop [i 0 acc 0]
    (if (= i 1000)
      acc
      (recur (inc i) (+ acc (f i))))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

// From core/transient_test.go
func BenchmarkTransientVectorLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(loop [i 0 v []]
  (if (= i 100)
    (count v)
    (recur (inc i) (conj v i))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

// From core/typed_nth_test.go
func BenchmarkIRTypedGenericStringNthLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(loop [i 0 s "GGTATTTTAATTTATAGT" acc ""]
  (if (= i 128)
    (count acc)
    (recur (inc i) s (str acc (nth s (rem i 18))))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

// From core/transient_string_test.go
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

// From core/transient_string_test.go
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

// From core/string_cursor_parse_test.go
// From core/string_cursor_parse_test.go
// From core/persistent_vector_test.go
func BenchmarkPVAssoc35(b *testing.B) {
	pv := corecollections.EmptyPersistentVector()
	for i := 0; i < 35; i++ {
		pv = pv.Conjoin(coretypes.Double{D: float64(i)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := pv
		for j := 0; j < 9; j++ {
			v = v.AssocIndex(j*3+3, coretypes.Double{D: float64(i + j)})
		}
		_ = v
	}
}

// From core/persistent_vector_test.go
func BenchmarkArrayVectorAssoc35(b *testing.B) {
	arr := make([]coretypes.Object, 35)
	for i := range arr {
		arr[i] = coretypes.Double{D: float64(i)}
	}
	av := corecollections.NewArrayVectorFrom(arr...)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v coretypes.Associative = av
		for j := 0; j < 9; j++ {
			v = v.Assoc(coretypes.MakeInt(j*3+3), coretypes.Double{D: float64(i + j)})
		}
		_ = v
	}
}

// From core/typed_values_test.go
func BenchmarkIRTypedStringLoop(b *testing.B) {
	expr := compileBenchExpr(b, `(let [dna "GGTATTTTAATTTATAGT"]
  (loop [i 0 s ""]
    (if (= i 128)
      (count s)
      (recur (inc i) (str s (nth dna (rem i 18)))))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

// From core/mem_array_test.go
func BenchmarkWasmArrayF64Sum(b *testing.B) {
	arr := corewasm.MakeF64ArrayWithRuntime(func() wazero.Runtime { return wazero.NewRuntime(nil) }, 10000, TYPE.ArrayVector)
	if arr == nil {
		b.Skip("WASM array failed")
	}
	for i := 0; i < 10000; i++ {
		arr.SetF64(i, float64(i)*0.1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		sum := 0.0
		for i := 0; i < 10000; i++ {
			sum += arr.GetF64(i)
		}
		_ = sum
	}
}

// From core/mem_array_test.go
func BenchmarkGoSliceF64Sum(b *testing.B) {
	arr := make([]float64, 10000)
	for i := range arr {
		arr[i] = float64(i) * 0.1
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		sum := 0.0
		for i := 0; i < 10000; i++ {
			sum += arr[i]
		}
		_ = sum
	}
}

// From core/wasm_compile_test.go
// From core/wasm_compile_test.go
// From core/typed_map_test.go
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

// From core/typed_helpers_test.go
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

// From core/inline_collection_test.go
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
