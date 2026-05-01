package core

import "testing"

func TestCharToStringFast(t *testing.T) {
	if got := charToStringFast('A'); got != "A" {
		t.Fatalf("expected A, got %q", got)
	}
	if got := charToStringFast('é'); got != "é" {
		t.Fatalf("expected é, got %q", got)
	}
	if got := charToStringObjectFast('A'); got.(String).S != "A" {
		t.Fatalf("expected cached A object, got %T %s", got, got.ToString(false))
	}
	if got := charToStringObjectFast('é'); got.(String).S != "é" {
		t.Fatalf("expected unicode string object, got %T %s", got, got.ToString(false))
	}
}
