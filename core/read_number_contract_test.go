package core

import (
	"strconv"
	"testing"
)

func TestReadIntegerUsesNativeIntRange(t *testing.T) {
	got := readOneForContract(t, "1099511627776")
	if strconv.IntSize == 32 {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("32-bit integer literal type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("64-bit integer literal type = %s, want Int", got.GetType().ToString(false))
	}
}
