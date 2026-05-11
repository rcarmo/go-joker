package system

import (
	"runtime"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestSystemProperties(t *testing.T) {
	props := systemProperties()
	if ok, v := props.Get(MakeString("os.name")); !ok || v.ToString(false) != runtime.GOOS {
		t.Fatalf("os.name mismatch: %v", v)
	}
	if ok, v := props.Get(MakeString("file.encoding")); !ok || v.ToString(false) != "UTF-8" {
		t.Fatalf("file.encoding mismatch: %v", v)
	}
}

func TestGetPropertyDefaultAndEnv(t *testing.T) {
	if v := getProperty([]Object{MakeString("missing"), MakeString("fallback")}); v.ToString(false) != "fallback" {
		t.Fatalf("default mismatch: %v", v)
	}
	t.Setenv("GO_JOKER_SYSTEM_TEST", "ok")
	if v := systemGetenv([]Object{MakeString("GO_JOKER_SYSTEM_TEST")}); v.ToString(false) != "ok" {
		t.Fatalf("env mismatch: %v", v)
	}
}

func TestTimes(t *testing.T) {
	if currentTimeMillis().(Int).I <= 0 || nanoTime().(Int).I <= 0 {
		t.Fatal("expected positive times")
	}
}
