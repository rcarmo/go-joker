package strconv

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestStrconvIntObjectPromotesOutsideNativeRange(t *testing.T) {
	got := strconvIntObject(math.MaxInt64)
	maxNativeInt := int64(int(^uint(0) >> 1))
	if math.MaxInt64 > maxNativeInt {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("strconv integer object type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("strconv integer object type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestStrconvIntObjectKeepsSmallValuesAsInt(t *testing.T) {
	got := strconvIntObject(42)
	if !got.Equals(coretypes.MakeInt(42)) {
		t.Fatalf("strconv integer object = %s, want 42", got.ToString(false))
	}
}
