package string

import "testing"

func TestGoName(t *testing.T) {
	if got := GoName("foo/bar?"); got != "foo_SLASH_bar_Q_" {
		t.Fatalf("GoName() = %q", got)
	}
}

func TestFilenameBracketHelpers(t *testing.T) {
	if got := FilenameUnbracketed("<joker.core>"); got != "joker.core" {
		t.Fatalf("FilenameUnbracketed() = %q", got)
	}
	if got := CoreNamespaceName("<joker.core>"); got != "joker.core" {
		t.Fatalf("CoreNamespaceName() = %q", got)
	}
}
