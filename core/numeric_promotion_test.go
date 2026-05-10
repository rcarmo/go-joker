package core

import (
	"strings"
	"testing"
)

func TestIntArithmeticPromotesToBigIntOnOverflow(t *testing.T) {
	if got := procAdd([]Object{Int{I: maxInt}, Int{I: 1}}); got.GetType() != TYPE.BigInt || got.ToString(false) != "9223372036854775808N" {
		t.Fatalf("add promotion mismatch: %T %s", got, got.ToString(false))
	}
	if got := procSubtract([]Object{Int{I: minInt}, Int{I: 1}}); got.GetType() != TYPE.BigInt || got.ToString(false) != "-9223372036854775809N" {
		t.Fatalf("subtract promotion mismatch: %T %s", got, got.ToString(false))
	}
	if got := procMultiply([]Object{Int{I: maxInt}, Int{I: 2}}); got.GetType() != TYPE.BigInt || got.ToString(false) != "18446744073709551614N" {
		t.Fatalf("multiply promotion mismatch: %T %s", got, got.ToString(false))
	}
}

func TestIncDecPromoteToBigIntOnOverflow(t *testing.T) {
	if got := procInc([]Object{Int{I: maxInt}}); got.GetType() != TYPE.BigInt {
		t.Fatalf("inc did not promote: %T %s", got, got.ToString(false))
	}
	if got := procDec([]Object{Int{I: minInt}}); got.GetType() != TYPE.BigInt {
		t.Fatalf("dec did not promote: %T %s", got, got.ToString(false))
	}
}

func TestBigDecimalArithmeticKeepsBigFloat(t *testing.T) {
	a, _ := MakeBigFloatWithOrig("0.1", "0.1M")
	b, _ := MakeBigFloatWithOrig("0.2", "0.2M")
	got := procAdd([]Object{a, b})
	if got.GetType() != TYPE.BigFloat || !strings.HasPrefix(got.ToString(false), "0.3") || !strings.HasSuffix(got.ToString(false), "M") {
		t.Fatalf("big decimal add mismatch: %T %s", got, got.ToString(false))
	}
}
