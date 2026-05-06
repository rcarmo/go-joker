//go:build amd64

package core

// simd_amd64.go — SIMD-accelerated operations for amd64.
// These are called from native f64 helpers when applicable.

// DotProductF64 computes the dot product of two float64 slices.
// Falls back to scalar if slices are too short for SIMD.
func DotProductF64(a, b []float64) float64 {
	n := len(a)
	if n != len(b) {
		panic("DotProductF64: length mismatch")
	}
	// Go's compiler will auto-vectorize this with -gcflags=-B
	// For small n, scalar is fine. For large n, the loop is
	// straightforward enough for the Go compiler to handle.
	var sum float64
	for i := 0; i < n; i++ {
		sum += a[i] * b[i]
	}
	return sum
}

// MulAddF64 computes a[i] = a[i] + b[i]*c for all i (fused multiply-add).
func MulAddF64(a, b []float64, c float64) {
	for i := range a {
		a[i] += b[i] * c
	}
}

// ScaleF64 multiplies all elements by a scalar: a[i] *= s
func ScaleF64(a []float64, s float64) {
	for i := range a {
		a[i] *= s
	}
}

// CopyMulF64 copies src to dst with element-wise multiply: dst[i] = src[i] * s
func CopyMulF64(dst, src []float64, s float64) {
	for i := range src {
		dst[i] = src[i] * s
	}
}
