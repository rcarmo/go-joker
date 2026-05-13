package ir

import "testing"

func TestFloat64Identity(t *testing.T) {
	in := []float64{1, 2, 3}
	out := Float64(in)
	if len(out) != 3 || out[0] != 1 || out[2] != 3 {
		t.Fatalf("Float64 identity failed: %#v", out)
	}
}
