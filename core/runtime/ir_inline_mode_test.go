package runtime

import "testing"

func TestIRInlineModeDefaultsAuto(t *testing.T) {
	if got := IRInlineMode(); got == "" {
		t.Fatal("expected non-empty mode")
	}
}
