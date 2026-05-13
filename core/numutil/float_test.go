package numutil

import "testing"

func TestComputeFloatPrecisionDefaultsToFloat64Precision(t *testing.T) {
	if got := ComputeFloatPrecision("1.5"); got < 53 {
		t.Fatalf("ComputeFloatPrecision() = %d, want >= 53", got)
	}
}

func TestNeedsDecimalSuffix(t *testing.T) {
	if !NeedsDecimalSuffix("1") {
		t.Fatal("expected integer-looking float rendering to need suffix")
	}
	if NeedsDecimalSuffix("1.5") {
		t.Fatal("did not expect decimal rendering to need suffix")
	}
	if NeedsDecimalSuffix("1e10") {
		t.Fatal("did not expect exponent rendering to need suffix")
	}
}
