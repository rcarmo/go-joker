package reader

import "testing"

func TestFilenameOrDefault(t *testing.T) {
	name := "source.clj"
	if got := FilenameOrDefault(&name); got != name {
		t.Fatalf("FilenameOrDefault named = %q", got)
	}
	if got := FilenameOrDefault(nil); got != "<file>" {
		t.Fatalf("FilenameOrDefault nil = %q", got)
	}
}
