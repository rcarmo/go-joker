package wasm

import "testing"

func TestTwoFuncExecModule(t *testing.T) {
	caller := []byte{0x00, 0x42, 0x00, 0x0b}
	helper := []byte{0x00, 0x42, 0x01, 0x0b}
	bin := TwoFuncExecModule(1, 1, ValTypeI64, caller, helper)
	if len(bin) == 0 {
		t.Fatal("expected non-empty module")
	}
}

func TestMemoryExportModule(t *testing.T) {
	caller := []byte{0x00, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0b}
	bin := MemoryExportModule(1, 0, caller, nil)
	if len(bin) == 0 {
		t.Fatal("expected non-empty module")
	}
}
