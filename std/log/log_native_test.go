package log

import (
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func expectLogPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestParseLogLevelAcceptsStringsAndKeywords(t *testing.T) {
	if got := parseLogLevel(MakeKeyword("debug"), "test"); got != 0 {
		t.Fatalf("keyword level = %d", got)
	}
	if got := parseLogLevel(MakeString("error"), "test"); got != 3 {
		t.Fatalf("string level = %d", got)
	}
}

func TestLogfChecksArity(t *testing.T) {
	initLogNamespace()
	logf := logNamespace.Resolve("logf").Resolve().(Callable)
	expectLogPanic(t, func() { logf.Call([]Object{MakeKeyword("info")}) })
}
