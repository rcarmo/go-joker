package runtime

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func TestReducedWrapCheckDeref(t *testing.T) {
	value := coretypes.MakeInt(42)
	reduced := MakeReduced(value)
	if !IsReduced(reduced) {
		t.Fatal("MakeReduced value should be reduced")
	}
	if got := DerefReduced(reduced); !got.Equals(value) {
		t.Fatalf("DerefReduced = %s, want %s", got.ToString(false), value.ToString(false))
	}
	if got := DerefReduced(value); !got.Equals(value) {
		t.Fatalf("DerefReduced non-reduced = %s, want %s", got.ToString(false), value.ToString(false))
	}
	if EnsureReduced(reduced) != reduced {
		t.Fatal("EnsureReduced should preserve existing Reduced wrapper")
	}
}
