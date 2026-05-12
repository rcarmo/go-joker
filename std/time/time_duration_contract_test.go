package time

import (
	"math"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestTimeIntObjectPromotesOutsideNativeRange(t *testing.T) {
	got := timeIntObject(math.MaxInt64)
	maxNativeInt := int64(int(^uint(0) >> 1))
	if math.MaxInt64 > maxNativeInt {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("time integer object type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("time integer object type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestTimeIntObjectKeepsSmallValuesAsInt(t *testing.T) {
	got := timeIntObject(42)
	if !got.Equals(MakeInt(42)) {
		t.Fatalf("time integer object = %s, want 42", got.ToString(false))
	}
}
