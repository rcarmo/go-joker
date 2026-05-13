package runtime

import "testing"

func TestNextWasmModuleName(t *testing.T) {
	wasmModMu.Lock()
	wasmModSeq = 98
	wasmModMu.Unlock()
	if got := NextWasmModuleName(); got != "joker_wasm_99" {
		t.Fatalf("first name = %q", got)
	}
	if got := NextWasmModuleName(); got != "joker_wasm_100" {
		t.Fatalf("second name = %q", got)
	}
}
