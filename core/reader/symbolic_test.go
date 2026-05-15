package reader

import (
	"math"
	"testing"
)

func TestSymbolicValue(t *testing.T) {
	if got, ok := SymbolicValue("Inf"); !ok || !math.IsInf(got, 1) {
		t.Fatalf("SymbolicValue(Inf) = %v/%v", got, ok)
	}
	if got, ok := SymbolicValue("-Inf"); !ok || !math.IsInf(got, -1) {
		t.Fatalf("SymbolicValue(-Inf) = %v/%v", got, ok)
	}
	if got, ok := SymbolicValue("NaN"); !ok || !math.IsNaN(got) {
		t.Fatalf("SymbolicValue(NaN) = %v/%v", got, ok)
	}
	if got, ok := SymbolicValue("other"); ok || got != 0 {
		t.Fatalf("SymbolicValue(other) = %v/%v, want 0/false", got, ok)
	}
}
