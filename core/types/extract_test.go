package types

import "testing"

func TestExtractHelpers(t *testing.T) {
	args := []Object{MakeString("x"), MakeInt(3), Boolean{B: true}}
	if got := ExtractString(args, 0); got != "x" {
		t.Fatalf("ExtractString = %q, want x", got)
	}
	if got := ExtractInt(args, 1); got != 3 {
		t.Fatalf("ExtractInt = %d, want 3", got)
	}
	if got := ExtractInteger(args, 1); got != 3 {
		t.Fatalf("ExtractInteger = %d, want 3", got)
	}
	if got := ExtractBoolean(args, 2); !got {
		t.Fatal("ExtractBoolean = false, want true")
	}
	if got := ExtractStrings([]Object{MakeString("a"), MakeString("b")}, 0); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("ExtractStrings = %#v", got)
	}
}
