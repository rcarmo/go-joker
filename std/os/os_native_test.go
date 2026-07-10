package os

import (
	"io"
	"math"
	"runtime"
	"testing"
	"time"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"
)

func TestEnvironmentPreservesEmptyAndEqualsValues(t *testing.T) {
	t.Setenv("GO_JOKER_ENV_EMPTY", "")
	t.Setenv("GO_JOKER_ENV_EQUALS", "left=right=tail")
	values := env().(coretypes.Map)
	for key, want := range map[string]string{
		"GO_JOKER_ENV_EMPTY":  "",
		"GO_JOKER_ENV_EQUALS": "left=right=tail",
	} {
		ok, got := values.Get(coretypes.MakeString(key))
		if !ok || got.ToString(false) != want {
			t.Fatalf("environment %s = %#v, want %q", key, got, want)
		}
	}
}

func TestStartProcessReleasesChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell command")
	}
	pid := startProcess("sh", corecollections.EmptyArrayMap().Assoc(coretypes.MakeKeyword(STRINGS.Intern, "args"), corecollections.NewVectorFrom(coretypes.MakeString("-c"), coretypes.MakeString("exit 0"))).(coretypes.Map))
	if pid <= 0 {
		t.Fatalf("startProcess pid = %d", pid)
	}
	// Give the child a chance to exit; the important contract is that startProcess
	// has released its handle and does not require a later Wait to avoid leaks.
	time.Sleep(10 * time.Millisecond)
}

func TestStartProcessDefaultsToDiscardedOutput(t *testing.T) {
	if got := processOutputOrDiscard(nil); got != io.Discard {
		t.Fatalf("nil process output = %T, want io.Discard", got)
	}
}

func TestNativeIntObjectPromotesOutsideNativeRange(t *testing.T) {
	got := nativeIntObject(math.MaxInt64)
	if math.MaxInt64 > int64(int(^uint(0)>>1)) {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("native int object type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("native int object type = %s, want coretypes.Int", got.GetType().ToString(false))
	}
}

func TestNativeIntObjectKeepsSmallValuesAsInt(t *testing.T) {
	got := nativeIntObject(42)
	if !got.Equals(coretypes.MakeInt(42)) {
		t.Fatalf("native int object = %s, want 42", got.ToString(false))
	}
}
