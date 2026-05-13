package trace

import "testing"

func TestEnvConstructorsReturnTracerInstances(t *testing.T) {
	if NewFunctionTracerFromEnv() == nil || NewSymbolTracerFromEnv() == nil || NewIRProfileFromEnv() == nil {
		t.Fatal("expected non-nil trace helpers")
	}
}
