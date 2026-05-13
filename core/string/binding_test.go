package string

import "testing"

func TestIsIgnorableBindingName(t *testing.T) {
	cases := map[string]bool{
		"_x":     true,
		"&form1": true,
		"&env2":  true,
		"user":   false,
	}
	for in, want := range cases {
		if got := IsIgnorableBindingName(in); got != want {
			t.Fatalf("IsIgnorableBindingName(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestHasNamespaceSeparator(t *testing.T) {
	if !HasNamespaceSeparator("joker.core", '.') {
		t.Fatal("expected dot separator")
	}
	if HasNamespaceSeparator("jokercore", '.') {
		t.Fatal("did not expect separator")
	}
}
