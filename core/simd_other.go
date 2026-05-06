//go:build !amd64

package core

// simd_arm64.go — scalar fallbacks for arm64 (same implementations,
// Go compiler may auto-vectorize with NEON on arm64).

// DotProductF64 computes the dot product of two float64 slices.
func DotProductF64(a, b []float64) float64 {
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// MulAddF64 computes a[i] = a[i] + b[i]*c for all i.
func MulAddF64(a, b []float64, c float64) {
	for i := range a {
		a[i] += b[i] * c
	}
}

// ScaleF64 multiplies all elements by a scalar.
func ScaleF64(a []float64, s float64) {
	for i := range a {
		a[i] *= s
	}
}

// CopyMulF64 copies src to dst with element-wise multiply.
func CopyMulF64(dst, src []float64, s float64) {
	for i := range src {
		dst[i] = src[i] * s
	}
}
