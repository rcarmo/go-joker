package ir

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math"
	"testing"
)

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

func TestNaNBoxObjectNumericBoundaries(t *testing.T) {
	for _, n := range []int{coretypes.MinInt, -2147483648, 2147483647, coretypes.MaxInt} {
		var table []coretypes.Object
		boxed := NBFromObject(coretypes.MakeInt(n), &table, nil)
		got := NBToObject(boxed, table, nil)
		v, ok := got.(coretypes.Int)
		if !ok || v.I != n {
			t.Fatalf("%d roundtrip became %T %v", n, got, got)
		}
	}
	for _, bits := range []uint64{0x7ff8000000000000, 0x7ff8000100000001, 0xfff8000000000001} {
		var table []coretypes.Object
		boxed := NBFromObject(coretypes.Double{D: math.Float64frombits(bits)}, &table, nil)
		got := NBToObject(boxed, table, nil)
		v, ok := got.(coretypes.Double)
		if !ok || !math.IsNaN(v.D) {
			t.Fatalf("NaN roundtrip became %T %v", got, got)
		}
	}
}

func FuzzNaNBoxNumericRoundTrip(f *testing.F) {
	for _, n := range []int64{0, 1, -1, 2147483647, 2147483648, -2147483649, 9223372036854775807} {
		f.Add(n, uint64(n))
	}
	f.Fuzz(func(t *testing.T, n int64, bits uint64) {
		var table []coretypes.Object
		value := coretypes.MakeInt(int(n))
		got := NBToObject(NBFromObject(value, &table, nil), table, nil)
		if !value.Equals(got) {
			t.Fatalf("integer %v became %v", value, got)
		}
		table = nil
		d := math.Float64frombits(bits)
		got = NBToObject(NBFromObject(coretypes.Double{D: d}, &table, nil), table, nil)
		roundtrip, ok := got.(coretypes.Double)
		if !ok {
			t.Fatalf("double became %T", got)
		}
		if math.IsNaN(d) {
			if !math.IsNaN(roundtrip.D) {
				t.Fatal("NaN lost")
			}
		} else if math.Float64bits(roundtrip.D) != bits {
			t.Fatal("double bits changed")
		}
	})
}
