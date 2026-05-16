package reader

import "testing"

func TestClassifyMissingTaggedReaderAction(t *testing.T) {
	cases := []struct {
		suppress bool
		linter   bool
		edn      bool
		want     MissingTaggedReaderAction
	}{
		{true, false, false, MissingTaggedReaderReturnValue},
		{false, true, false, MissingTaggedReaderWarnAndReturnValue},
		{false, true, true, MissingTaggedReaderReturnValue},
		{false, false, false, MissingTaggedReaderError},
	}
	for _, tc := range cases {
		if got := ClassifyMissingTaggedReaderAction(tc.suppress, tc.linter, tc.edn); got != tc.want {
			t.Fatalf("ClassifyMissingTaggedReaderAction(%v, %v, %v) = %v, want %v", tc.suppress, tc.linter, tc.edn, got, tc.want)
		}
	}
}

func TestTaggedLiteralPrefix(t *testing.T) {
	if got := TaggedLiteralPrefix("foo/bar"); got != "#foo/bar " {
		t.Fatalf("TaggedLiteralPrefix = %q", got)
	}
}
