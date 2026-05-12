package core

import (
	"math/big"
	"strconv"
	"testing"
)

func TestRatioOrIntUsesNativeIntRange(t *testing.T) {
	tooLargeFor32Bit := new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 40))
	got := ratioOrInt(tooLargeFor32Bit)
	if strconv.IntSize == 32 {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("32-bit ratio integer promotion type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("64-bit ratio integer promotion type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestRatioOrIntWithOriginalPreservesBigIntOriginal(t *testing.T) {
	tooLarge := new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 70))
	got := ratioOrIntWithOriginal("1180591620717411303424/1", tooLarge)
	bi, ok := got.(*BigInt)
	if !ok {
		t.Fatalf("large ratio integer type = %T, want *BigInt", got)
	}
	if bi.Original != "1180591620717411303424/1" {
		t.Fatalf("BigInt original = %q", bi.Original)
	}
}
