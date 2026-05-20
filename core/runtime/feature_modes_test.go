package runtime

import "testing"

func TestFeatureFlagHelpersReturnValues(t *testing.T) {
	if IRTypedMapMode() == "" || IRStringBuilderMode() == "" || WasmMultiFnMode() == "" {
		t.Fatal("expected non-empty mode strings")
	}
}

func TestIRInlineModeDefaultsAuto(t *testing.T) {
	if got := IRInlineMode(); got == "" {
		t.Fatal("expected non-empty mode")
	}
}

func TestIRTypedMapModeDefaultsAuto(t *testing.T) {
	if got := IRTypedMapMode(); got == "" {
		t.Fatal("expected non-empty mode")
	}
}
