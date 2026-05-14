package string

import (
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestPadLeftRightUseRuneWidth(t *testing.T) {
	if got := padRight("å", ".", 3); got != "å.." {
		t.Fatalf("padRight unicode result = %q, want å..", got)
	}
	if got := padLeft("å", "ab", 4); got != "babå" {
		t.Fatalf("padLeft unicode result = %q, want babå", got)
	}
}

func TestSplitOnEmptyStringUsesWhitespaceFields(t *testing.T) {
	got := splitOnStringOrRegex(" alpha  beta\tgamma ", String{S: ""}, -1).(interface {
		Count() int
		Nth(int) Object
	})
	if got.Count() != 3 {
		t.Fatalf("split field count = %d, want 3", got.Count())
	}
	if got.Nth(1).(String).S != "beta" {
		t.Fatalf("second split field = %q, want beta", got.Nth(1))
	}
}

func TestIsBlank(t *testing.T) {
	if !isBlank(String{S: " \t\n"}) || isBlank(String{S: " x "}) || !isBlank(NIL) {
		t.Fatal("isBlank boundary behavior mismatch")
	}
}
