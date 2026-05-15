package reader

import "testing"

func TestAnalyzeNumberToken(t *testing.T) {
	tests := []struct {
		in     string
		kind   NumberTokenKind
		digits string
		base   int
	}{
		{in: "42", kind: NumberTokenInt, digits: "42", base: 0},
		{in: "3.14", kind: NumberTokenFloat, digits: "3.14", base: 0},
		{in: "1e9", kind: NumberTokenFloat, digits: "1e9", base: 0},
		{in: "1/2", kind: NumberTokenRatio, digits: "1/2", base: 0},
		{in: "42N", kind: NumberTokenBigInt, digits: "42", base: 0},
		{in: "42M", kind: NumberTokenBigFloat, digits: "42", base: 0},
		{in: "16rff", kind: NumberTokenInt, digits: "ff", base: 16},
		{in: "-16rff", kind: NumberTokenInt, digits: "-ff", base: 16},
	}
	for _, tt := range tests {
		got, err := AnalyzeNumberToken(tt.in)
		if err != nil {
			t.Fatalf("AnalyzeNumberToken(%q) error: %v", tt.in, err)
		}
		if got.Kind != tt.kind || got.Digits != tt.digits || got.Base != tt.base || got.Original != tt.in {
			t.Fatalf("AnalyzeNumberToken(%q) = %#v, want kind %v digits %q base %d", tt.in, got, tt.kind, tt.digits, tt.base)
		}
	}
}

func TestAnalyzeNumberTokenInvalid(t *testing.T) {
	for _, in := range []string{"", "N", "M", "1r1", "37r1", "1/2/3"} {
		if _, err := AnalyzeNumberToken(in); err == nil {
			t.Fatalf("AnalyzeNumberToken(%q) succeeded, want error", in)
		}
	}
}
