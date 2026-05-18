package string

import "testing"

func TestCompare(t *testing.T) {
	if Compare("a", "b") >= 0 {
		t.Fatal("expected a < b")
	}
	if Compare("b", "a") <= 0 {
		t.Fatal("expected b > a")
	}
	if Compare("a", "a") != 0 {
		t.Fatal("expected equal strings to compare as 0")
	}
}
