package system

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math"
	"runtime"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestSystemProperties(t *testing.T) {
	props := systemProperties()
	if ok, v := props.Get(coretypes.MakeString("os.name")); !ok || v.ToString(false) != runtime.GOOS {
		t.Fatalf("os.name mismatch: %v", v)
	}
	if ok, v := props.Get(coretypes.MakeString("file.encoding")); !ok || v.ToString(false) != "UTF-8" {
		t.Fatalf("file.encoding mismatch: %v", v)
	}
}

func TestGetPropertyDefaultAndEnv(t *testing.T) {
	if v := getProperty([]coretypes.Object{coretypes.MakeString("missing"), coretypes.MakeString("fallback")}); v.ToString(false) != "fallback" {
		t.Fatalf("default mismatch: %v", v)
	}
	t.Setenv("GO_JOKER_SYSTEM_TEST", "ok")
	if v := systemGetenv([]coretypes.Object{coretypes.MakeString("GO_JOKER_SYSTEM_TEST")}); v.ToString(false) != "ok" {
		t.Fatalf("env mismatch: %v", v)
	}
}

func TestTimes(t *testing.T) {
	if currentTimeMillis().(coretypes.Number).BigInt().Sign() <= 0 || nanoTime().(coretypes.Number).BigInt().Sign() <= 0 {
		t.Fatal("expected positive times")
	}
}

func TestSystemIntObjectPromotesOutsideNativeRange(t *testing.T) {
	got := systemIntObject(math.MaxInt64)
	if math.MaxInt64 > int64(int(^uint(0)>>1)) {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("system integer object type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("system integer object type = %s, want Int", got.GetType().ToString(false))
	}
}
