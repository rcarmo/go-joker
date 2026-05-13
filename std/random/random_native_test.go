package random

import (
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func expectRandomPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestRandomIntBetweenRejectsOverflowRange(t *testing.T) {
	initRandomNamespace()
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	expectRandomPanic(t, func() {
		intBetween := randomNamespace.Resolve("int-between").Resolve().(Callable)
		intBetween.Call([]Object{MakeInt(minInt), MakeInt(maxInt)})
	})
}

func TestRandomSecureArgsValidate(t *testing.T) {
	initRandomNamespace()
	expectRandomPanic(t, func() {
		secureBytes := randomNamespace.Resolve("secure-bytes").Resolve().(Callable)
		secureBytes.Call([]Object{MakeInt(0)})
	})
	expectRandomPanic(t, func() {
		secureInt := randomNamespace.Resolve("secure-int").Resolve().(Callable)
		secureInt.Call([]Object{MakeInt(0)})
	})
}
