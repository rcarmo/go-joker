package types

import "testing"

func TestCharStringFast(t *testing.T) {
	if got := CharToStringObjectFast('x'); got.ToString(false) != "x" {
		t.Fatalf("CharToStringObjectFast = %s, want x", got.ToString(false))
	}
}
