package math

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	stdmath "math"
	"testing"
)

func TestModfBoundary(t *testing.T) {
	got := modf(3.75).(interface {
		Count() int
		Nth(int) coretypes.Object
	})
	if got.Count() != 2 {
		t.Fatalf("modf returned %d values, want 2", got.Count())
	}
	if i := got.Nth(0).(coretypes.Double).D; i != 3 {
		t.Fatalf("integer part = %f, want 3", i)
	}
	if f := got.Nth(1).(coretypes.Double).D; stdmath.Abs(f-0.75) > 1e-12 {
		t.Fatalf("fractional part = %f, want 0.75", f)
	}
}

func TestSetPrecisionRejectsNegativePrecision(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("setPrecision accepted negative precision")
		}
	}()
	setPrecision(coretypes.MakeInt(-1), coretypes.MakeDouble(1.25).BigFloat())
}
