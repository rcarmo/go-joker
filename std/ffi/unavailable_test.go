//go:build (!linux && !darwin && !windows) || (!amd64 && !arm64)

package ffi

import (
	core "github.com/rcarmo/go-joker/v42/core"
	"testing"
)

func TestFFIDisabled(t *testing.T) {
	unavailable()
	for _, name := range []string{"open", "bind", "buffer", "u32"} {
		v := namespace.Resolve(name)
		if v == nil {
			t.Fatalf("missing documented var %s", name)
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("disabled %s ran", name)
				}
			}()
			v.Value.(core.Proc).Call(nil)
		}()
	}
}
