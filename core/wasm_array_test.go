package core

import "testing"

func TestWasmArrayF64(t *testing.T) {
	arr := MakeF64Array(10)
	if arr == nil {
		t.Skip("WASM array allocation failed")
	}
	arr.SetF64(0, 3.14)
	arr.SetF64(5, 2.71)
	if arr.GetF64(0) != 3.14 {
		t.Fatalf("expected 3.14, got %f", arr.GetF64(0))
	}
	if arr.GetF64(5) != 2.71 {
		t.Fatalf("expected 2.71, got %f", arr.GetF64(5))
	}
	if arr.GetF64(3) != 0 {
		t.Fatalf("expected 0, got %f", arr.GetF64(3))
	}
	if arr.Length() != 10 {
		t.Fatalf("expected length 10, got %d", arr.Length())
	}
}

func BenchmarkWasmArrayF64Sum(b *testing.B) {
	arr := MakeF64Array(10000)
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
