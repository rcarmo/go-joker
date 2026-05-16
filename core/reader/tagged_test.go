package reader

import "testing"

func TestDataReaderVarNames(t *testing.T) {
	names := DataReaderVarNames()
	if len(names) != 2 || names[0] != "*data-readers*" || names[1] != "default-data-readers" {
		t.Fatalf("DataReaderVarNames = %#v", names)
	}
	names[0] = "mutated"
	if DataReaderVarNames()[0] != "*data-readers*" {
		t.Fatal("DataReaderVarNames exposed mutable package state")
	}
}

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
