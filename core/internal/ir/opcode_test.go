package ir

import (
	"strings"
	"testing"
)

func TestOpcodeName(t *testing.T) {
	if got := OpcodeName(Add); got != "irAdd" {
		t.Fatalf("OpcodeName(Add) = %q", got)
	}
	if got := OpcodeName(255); got != "irUnknown(255)" {
		t.Fatalf("OpcodeName(255) = %q", got)
	}
}

func TestOpCountVariableWidthInstructions(t *testing.T) {
	code := []byte{
		Literal, 0, 0,
		CallSlot, 0, 1, 0, 2,
		Recur, 0, 2, 0, 9, 0, 3,
		Return,
	}
	if got := OpCount(code); got != 4 {
		t.Fatalf("OpCount = %d, want 4", got)
	}
}

func TestDisassemble(t *testing.T) {
	code := []byte{
		Literal, 0, 1,
		LoadSlot, 0, 2,
		CallSelf, 0, 1,
		Return,
	}
	got := Disassemble(code, func(idx int) string {
		if idx == 1 {
			return "42"
		}
		return ""
	})
	for _, want := range []string{"[  0] irLiteral const[1]=42", "[  3] irLoadSlot slot[2]", "[  6] irCallSelf nargs=1", "[  9] irReturn"} {
		if !strings.Contains(got, want) {
			t.Fatalf("disassembly missing %q:\n%s", want, got)
		}
	}
}
