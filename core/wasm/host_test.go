package wasm

import "testing"

func TestStandardHostImports(t *testing.T) {
	want := []string{"get", "get3", "assoc", "nth", "conj", "count", "first"}
	got := HostImportNames(StandardHostImports)
	if len(got) != len(want) {
		t.Fatalf("host import count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("host import[%d] = %q, want %q", i, got[i], want[i])
		}
		if StandardHostImports[i].NumParams <= 0 {
			t.Fatalf("host import[%d] has invalid param count %d", i, StandardHostImports[i].NumParams)
		}
	}
}
