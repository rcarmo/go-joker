package reader

import "testing"

func TestIdentValidationReason(t *testing.T) {
	if got := IdentValidationReason('x', true, "set", true, "range"); got != "" {
		t.Fatalf("IdentValidationReason pass = %q, want empty", got)
	}
	if got := IdentValidationReason('x', true, "set", false, "range"); got != "range" {
		t.Fatalf("IdentValidationReason range fail = %q", got)
	}
	if got := IdentValidationReason('x', false, "set", true, "range"); got != "set" {
		t.Fatalf("IdentValidationReason set fail = %q", got)
	}
	if got := IdentValidationReason('x', false, "set", false, "range"); got != "set; range" {
		t.Fatalf("IdentValidationReason both fail = %q", got)
	}
}

func TestFindIdentValidationIssues(t *testing.T) {
	text := "a b"
	issues := FindIdentValidationIssues(&text, IsValidCoreRune, ValidCoreReason, IsValidASCIIRune, ValidASCIIReason)
	if len(issues) != 1 || issues[0].Rune != ' ' || issues[0].Index != 1 || issues[0].Reason == "" {
		t.Fatalf("FindIdentValidationIssues = %#v", issues)
	}
	if got := FindIdentValidationIssues(nil, IsValidCoreRune, ValidCoreReason, IsValidASCIIRune, ValidASCIIReason); got != nil {
		t.Fatalf("FindIdentValidationIssues nil = %#v", got)
	}
}

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
	if !IsValidVisibleRune('_') || !IsValidUnicodeRune('\U0010ffff') || IsValidUnicodeRune(-1) || !IsValidASCIIRune('A') || IsValidASCIIRune('λ') || !IsValidAnyRune(-1) {
		t.Fatal("unexpected validation helper result")
	}
}
