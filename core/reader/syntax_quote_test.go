package reader

import "testing"

func TestAutoGensymSymbolName(t *testing.T) {
	if !IsAutoGensymSymbolName("foo#", false) {
		t.Fatal("foo# without namespace should be auto-gensym")
	}
	if IsAutoGensymSymbolName("foo#", true) {
		t.Fatal("namespaced foo# should not be auto-gensym")
	}
	if IsAutoGensymSymbolName("foo", false) {
		t.Fatal("foo should not be auto-gensym")
	}
	if got := AutoGensymPrefix("foo#"); got != "foo__" {
		t.Fatalf("AutoGensymPrefix = %q, want foo__", got)
	}
}
