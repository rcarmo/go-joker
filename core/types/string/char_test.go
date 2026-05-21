package string

import "testing"

func TestCharToStringFast(t *testing.T) {
	if got := CharToStringFast('å'); got != "å" {
		t.Fatalf("CharToStringFast = %q, want å", got)
	}
}
