package string

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
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
	if got := padRight("x", "", 3); got != "x" {
		t.Fatalf("padRight empty pad = %q, want x", got)
	}
	if got := padLeft("x", "", 3); got != "x" {
		t.Fatalf("padLeft empty pad = %q, want x", got)
	}
}

func TestSplitOnEmptyStringUsesWhitespaceFields(t *testing.T) {
	got := splitOnStringOrRegex(" alpha  beta\tgamma ", coretypes.String{S: ""}, -1).(interface {
		Count() int
		Nth(int) Object
	})
	if got.Count() != 3 {
		t.Fatalf("split field count = %d, want 3", got.Count())
	}
	if got.Nth(1).(coretypes.String).S != "beta" {
		t.Fatalf("second split field = %q, want beta", got.Nth(1))
	}
}

func TestStringIndexBounds(t *testing.T) {
	if got := indexOf("åbc", coretypes.Char{Ch: 'b'}, -3); !got.Equals(coretypes.MakeInt(1)) {
		t.Fatalf("indexOf negative from = %v, want 1", got)
	}
	if got := indexOf("åbc", coretypes.Char{Ch: 'b'}, 10); !got.Equals(NIL) {
		t.Fatalf("indexOf oversized from = %v, want nil", got)
	}
	if got := lastIndexOf("ababa", coretypes.String{S: "ba"}, 99); !got.Equals(coretypes.MakeInt(3)) {
		t.Fatalf("lastIndexOf oversized from = %v, want 3", got)
	}
	if got := lastIndexOf("ababa", coretypes.String{S: "ba"}, -1); !got.Equals(NIL) {
		t.Fatalf("lastIndexOf negative from = %v, want nil", got)
	}
}

func TestIsBlank(t *testing.T) {
	if !isBlank(coretypes.String{S: " \t\n"}) || isBlank(coretypes.String{S: " x "}) || !isBlank(NIL) {
		t.Fatal("isBlank boundary behavior mismatch")
	}
}
