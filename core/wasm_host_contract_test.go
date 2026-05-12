package core

import (
	"math"
	"testing"
)

func TestWasmRawIntObjectPromotesOutsideNativeRange(t *testing.T) {
	got := wasmRawIntObject(uint64(math.MaxInt64))
	if math.MaxInt64 > int64(maxInt) {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("wasm raw int object type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("wasm raw int object type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestWasmRawIntRejectsOutOfRangeIndex(t *testing.T) {
	if _, ok := wasmRawInt(uint64(math.MaxInt64)); ok && math.MaxInt64 > int64(maxInt) {
		t.Fatal("wasmRawInt should reject values outside native int range")
	}
}
