package io

import (
	"math"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestCopyCountObjectPromotesOutsideNativeRange(t *testing.T) {
	got := copyCountObject(int64(math.MaxInt64))
	maxNativeInt := int64(int(^uint(0) >> 1))
	if int64(math.MaxInt64) > maxNativeInt {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("copy count type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("copy count type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestCopyCountObjectKeepsSmallCountsAsInt(t *testing.T) {
	got := copyCountObject(42)
	if !got.Equals(MakeInt(42)) {
		t.Fatalf("copy count = %s, want 42", got.ToString(false))
	}
}
