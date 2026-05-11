package core

import "testing"

func TestNextWasmModNameDoesNotWrapAtTwoDigits(t *testing.T) {
	wasmModMu.Lock()
	old := wasmModSeq
	wasmModSeq = 98
	wasmModMu.Unlock()

	defer func() {
		wasmModMu.Lock()
		wasmModSeq = old
		wasmModMu.Unlock()
	}()

	if got := nextWasmModName(); got != "joker_wasm_99" {
		t.Fatalf("first name = %q", got)
	}
	if got := nextWasmModName(); got != "joker_wasm_100" {
		t.Fatalf("second name = %q", got)
	}
}
