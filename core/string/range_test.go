package string

import "testing"

func TestIntRangeLabel(t *testing.T) {
	cases := map[string]string{
		IntRangeLabel(1, 1):   "1",
		IntRangeLabel(1, 2):   "1 or 2",
		IntRangeLabel(1, 3):   "1, 2, or 3",
		IntRangeLabel(2, 999): "at least 2",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("IntRangeLabel mismatch: got %q want %q", got, want)
		}
	}
}
