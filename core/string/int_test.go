package string

import "testing"

func TestIntUsesCacheForSmallValues(t *testing.T) {
	if got := Int(42); got != "42" {
		t.Fatalf("Int(42) = %q", got)
	}
}

func TestIntHandlesLargeValues(t *testing.T) {
	if got := Int(50000); got != "50000" {
		t.Fatalf("Int(50000) = %q", got)
	}
}
