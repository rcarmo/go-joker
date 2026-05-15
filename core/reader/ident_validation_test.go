package reader

import "testing"

func TestIdentValidationHelpers(t *testing.T) {
	for _, r := range []rune{'a', 'Z', '0', '*', '+', '!', '-', '?', '=', '<', '>', '&', '_', '.', '\'', '#', '$', ':', '%'} {
		if !IsCoreIdentRune(r) {
			t.Fatalf("IsCoreIdentRune(%q) = false, want true", r)
		}
	}
	if IsCoreIdentRune('/') {
		t.Fatal("IsCoreIdentRune('/') = true, want false")
	}
	if !IsValidCoreRune('λ') || !IsValidCoreRune('9') || IsValidCoreRune('?') {
		t.Fatal("unexpected IsValidCoreRune result")
	}
	if !IsValidSymbolRune('∑') || IsValidSymbolRune('_') {
		t.Fatal("unexpected IsValidSymbolRune result")
	}
	if !IsValidVisibleRune('_') || !IsValidUnicodeRune('\U0010ffff') || !IsValidASCIIRune('A') || IsValidASCIIRune('λ') || !IsValidAnyRune(-1) {
		t.Fatal("unexpected validation helper result")
	}
}
