package runtime

import (
	"math"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestRuntimeIntObjectPromotesOutsideNativeRange(t *testing.T) {
	got := runtimeIntObject(math.MaxInt64)
	if math.MaxInt64 > int64(int(^uint(0)>>1)) {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("runtime int object type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("runtime int object type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestRuntimeUintObjectPromotesOutsideNativeRange(t *testing.T) {
	got := runtimeUintObject(math.MaxUint64)
	if uint64(int(^uint(0)>>1)) < math.MaxUint64 {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("runtime uint object type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("runtime uint object type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestProfileRejectsNonPositiveIterations(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("profile should reject non-positive iterations")
		}
	}()
	procProfile([]Object{Proc{Fn: func(args []Object) Object { return NIL }}, MakeInt(0)})
}
