package random

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
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
		intBetween := randomNamespace.Resolve("int-between").Resolve().(coretypes.Callable)
		intBetween.Call([]coretypes.Object{coretypes.MakeInt(minInt), coretypes.MakeInt(maxInt)})
	})
}

func TestRandomSecureArgsValidate(t *testing.T) {
	initRandomNamespace()
	expectRandomPanic(t, func() {
		secureBytes := randomNamespace.Resolve("secure-bytes").Resolve().(coretypes.Callable)
		secureBytes.Call([]coretypes.Object{coretypes.MakeInt(0)})
	})
	expectRandomPanic(t, func() {
		secureInt := randomNamespace.Resolve("secure-int").Resolve().(coretypes.Callable)
		secureInt.Call([]coretypes.Object{coretypes.MakeInt(0)})
	})
}
