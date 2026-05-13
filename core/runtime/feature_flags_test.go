package runtime

import "testing"

func TestFeatureFlagHelpersReturnValues(t *testing.T) {
	if IRTypedMapMode() == "" || IRStringBuilderMode() == "" || WasmMultiFnMode() == "" {
		t.Fatal("expected non-empty mode strings")
	}
}
