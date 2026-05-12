package core

import (
	"math/big"
	"testing"
)

func TestBigIntIntPanicsOutsideNativeRange(t *testing.T) {
	tooLarge := MakeBigInt(new(big.Int).Add(maxIntBig, big.NewInt(1)))
	defer func() {
		if recover() == nil {
			t.Fatal("BigInt.Int should panic outside native int range")
		}
	}()
	_ = tooLarge.Int()
}

func TestBigIntIntConvertsWithinNativeRange(t *testing.T) {
	got := MakeBigInt(big.NewInt(42)).Int()
	if got.I != 42 {
		t.Fatalf("BigInt.Int = %d, want 42", got.I)
	}
}
