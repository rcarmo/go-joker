package ir

import "testing"

func TestNaNBoxRoundTrip(t *testing.T) {
	v := BoxInt(42)
	if !IsInt(v) || ToInt(v) != 42 {
		t.Fatalf("int roundtrip failed")
	}
	f := BoxDouble(3.5)
	if !IsDouble(f) || ToDouble(f) != 3.5 {
		t.Fatalf("double roundtrip failed")
	}
}
