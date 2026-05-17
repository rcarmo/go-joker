package os

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"
	"math"
	"runtime"
	"testing"
	"time"

	. "github.com/rcarmo/go-joker/core"
)

func TestStartProcessReleasesChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix shell command")
	}
	pid := startProcess("sh", EmptyArrayMap().Assoc(MakeKeyword("args"), NewVectorFrom(MakeString("-c"), MakeString("exit 0"))).(Map))
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
		t.Fatalf("native int object type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestNativeIntObjectKeepsSmallValuesAsInt(t *testing.T) {
	got := nativeIntObject(42)
	if !got.Equals(coretypes.MakeInt(42)) {
		t.Fatalf("native int object = %s, want 42", got.ToString(false))
	}
}
