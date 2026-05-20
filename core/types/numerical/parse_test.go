package numerical

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

func TestParseInt(t *testing.T) {
	got, err := ParseInt("42", 10, 64)
	if err != nil || got != 42 {
		t.Fatalf("ParseInt() = %d, %v", got, err)
	}
}

func TestParseFloat64(t *testing.T) {
	got, err := ParseFloat64("3.5")
	if err != nil || got != 3.5 {
		t.Fatalf("ParseFloat64() = %v, %v", got, err)
	}
}
