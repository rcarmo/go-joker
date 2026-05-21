package runtime

import (
	"errors"
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func TestPanicOnErr(t *testing.T) {
	old := coretypes.RuntimeError
	coretypes.RuntimeError = func(msg string) any { return errors.New(msg) }
	defer func() { coretypes.RuntimeError = old }()
	defer func() {
		if r := recover(); r == nil || r.(error).Error() != "boom" {
			t.Fatalf("panic = %#v, want boom error", r)
		}
	}()
	PanicOnErr(errors.New("boom"))
}
