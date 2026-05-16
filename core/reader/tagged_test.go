package reader

import "testing"

func TestTaggedLiteralPrefix(t *testing.T) {
	if got := TaggedLiteralPrefix("foo/bar"); got != "#foo/bar " {
		t.Fatalf("TaggedLiteralPrefix = %q", got)
	}
}
