package string

import "testing"

func TestTrimVarQuotePrefix(t *testing.T) {
	if got := TrimVarQuotePrefix("#'foo"); got != "foo" {
		t.Fatalf("TrimVarQuotePrefix() = %q", got)
	}
}

func TestHasJokerNamespacePrefix(t *testing.T) {
	if !HasJokerNamespacePrefix("joker.core") {
		t.Fatal("expected joker.core to have joker prefix")
	}
	if HasJokerNamespacePrefix("user") {
		t.Fatal("did not expect user to have joker prefix")
	}
}
