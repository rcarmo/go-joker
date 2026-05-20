package string

import (
	"reflect"
	"testing"
)

func TestIntRangeLabel(t *testing.T) {
	cases := map[string]string{
		IntRangeLabel(1, 1):   "1",
		IntRangeLabel(1, 2):   "1 or 2",
		IntRangeLabel(1, 3):   "1, 2, or 3",
		IntRangeLabel(1, 999): "at least 1",
		IntRangeLabel(2, 10):  "between 2 and 10, inclusive",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("IntRangeLabel = %q, want %q", got, want)
		}
	}
}

func TestSplitWhitespace(t *testing.T) {
	got := SplitWhitespace("a b\n\tc\r\nd")
	want := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitWhitespace() = %#v, want %#v", got, want)
	}
}

func TestParseVersionTriplet(t *testing.T) {
	major, minor, incremental := ParseVersionTriplet("v1.2.3")
	if major != 1 || minor != 2 || incremental != 3 {
		t.Fatalf("ParseVersionTriplet = %d.%d.%d", major, minor, incremental)
	}
	major, minor, incremental = ParseVersionTriplet("bad")
	if major != 0 || minor != 0 || incremental != 0 {
		t.Fatalf("ParseVersionTriplet bad = %d.%d.%d", major, minor, incremental)
	}
}
