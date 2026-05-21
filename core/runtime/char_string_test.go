package runtime

import "testing"

func TestCharStringFast(t *testing.T) {
	if got := CharToStringFast('å'); got != "å" {
		t.Fatalf("CharToStringFast = %q, want å", got)
	}
	if got := CharToStringObjectFast('x'); got.ToString(false) != "x" {
		t.Fatalf("CharToStringObjectFast = %s, want x", got.ToString(false))
	}
}
