package wasm

import (
	"bytes"
	"testing"
)

func TestModuleHeaderAndSections(t *testing.T) {
	m := NewModule()
	m.AddTypeSection(2)
	m.AddFuncSection()
	m.AddExportSection()
	m.AddCodeSection([]byte{0x00, 0x41, 0x00, 0x0b})
	b := m.Bytes()
	if !bytes.HasPrefix(b, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}) {
		t.Fatalf("missing wasm header: %x", b[:8])
	}
	for _, section := range []byte{0x01, 0x03, 0x07, 0x0a} {
		if !bytes.Contains(b, []byte{section}) {
			t.Fatalf("module missing section %x: %x", section, b)
		}
	}
}

func TestModuleImportSectionUsesHostModule(t *testing.T) {
	m := NewModule()
	m.AddImportSection("joker", []string{"get"})
	if !bytes.Contains(m.Bytes(), []byte("joker")) || !bytes.Contains(m.Bytes(), []byte("get")) {
		t.Fatalf("import section missing host/function names: %x", m.Bytes())
	}
}

func TestModuleBytesReturnsCopy(t *testing.T) {
	m := NewModule()
	b := m.Bytes()
	b[0] = 0xff
	if m.Bytes()[0] != 0x00 {
		t.Fatal("Bytes exposed mutable module buffer")
	}
}

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
