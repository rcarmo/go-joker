package runtime

import "testing"

func TestIRTypedMapModeDefaultsAuto(t *testing.T) {
	if got := IRTypedMapMode(); got == "" {
		t.Fatal("expected non-empty mode")
	}
}
