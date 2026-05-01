package core

import "testing"

func TestCharToStringFast(t *testing.T) {
	if got := charToStringFast('A'); got != "A" {
		t.Fatalf("expected A, got %q", got)
	}
	if got := charToStringFast('é'); got != "é" {
		t.Fatalf("expected é, got %q", got)
	}
}

func TestStringRuneCountFast(t *testing.T) {
	if got := stringRuneCountFast("abcdef"); got != 6 {
		t.Fatalf("expected 6, got %d", got)
	}
	if got := stringRuneCountFast("éclair"); got != 6 {
		t.Fatalf("expected 6 runes, got %d", got)
	}
	requireInt(t, evalTestScript(t, `(count "éclair")`), 6)
}

func TestSubsFastPathCorrectness(t *testing.T) {
	requireString(t, evalTestScript(t, `(subs "abcdef" 2 5)`), "cde")
	requireString(t, evalTestScript(t, `(subs "éclair" 1 4)`), "cla")
}
