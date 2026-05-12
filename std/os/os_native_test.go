package os

import (
	"math"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestNativeIntObjectPromotesOutsideNativeRange(t *testing.T) {
	got := nativeIntObject(math.MaxInt64)
	if math.MaxInt64 > int64(int(^uint(0)>>1)) {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("native int object type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("native int object type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestNativeIntObjectKeepsSmallValuesAsInt(t *testing.T) {
	got := nativeIntObject(42)
	if !got.Equals(MakeInt(42)) {
		t.Fatalf("native int object = %s, want 42", got.ToString(false))
	}
}
