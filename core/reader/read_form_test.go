package reader

import "testing"

func TestNeedsReadFormPeek(t *testing.T) {
	for _, r := range []rune{'.', '-', '+', '/'} {
		if !NeedsReadFormPeek(r) {
			t.Fatalf("NeedsReadFormPeek(%q) = false", r)
		}
	}
	for _, r := range []rune{'#', 'a', '1', EOF} {
		if NeedsReadFormPeek(r) {
			t.Fatalf("NeedsReadFormPeek(%q) = true", r)
		}
	}
}

func TestClassifyReadForm(t *testing.T) {
	cases := []struct {
		r, peek rune
		args    bool
		format  bool
		cljs    bool
		want    ReadFormKind
	}{
		{'\\', 0, false, false, false, ReadFormCharacter},
		{'1', 0, false, false, false, ReadFormNumber},
		{'.', '1', false, false, true, ReadFormNumber},
		{'.', '1', false, false, false, ReadFormIdent},
		{'-', '1', false, false, false, ReadFormNumber},
		{'%', 0, true, false, false, ReadFormArgSymbol},
		{'%', 0, true, true, false, ReadFormIdent},
		{'"', 0, false, false, false, ReadFormString},
		{'(', 0, false, false, false, ReadFormList},
		{'[', 0, false, false, false, ReadFormVector},
		{'{', 0, false, false, false, ReadFormMap},
		{'/', ' ', false, false, false, ReadFormStandaloneSlash},
		{'\'', 0, false, false, false, ReadFormQuote},
		{'@', 0, false, false, false, ReadFormDeref},
		{'~', 0, false, false, false, ReadFormUnquote},
		{'`', 0, false, false, false, ReadFormSyntaxQuote},
		{'^', 0, false, false, false, ReadFormMeta},
		{'#', 0, false, false, false, ReadFormDispatch},
		{EOF, 0, false, false, false, ReadFormEOF},
		{')', 0, false, false, false, ReadFormClosingDelimiter},
		{'x', 0, false, false, false, ReadFormIdent},
	}
	for _, tc := range cases {
		if got := ClassifyReadForm(tc.r, tc.peek, tc.args, tc.format, tc.cljs); got != tc.want {
			t.Fatalf("ClassifyReadForm(%q, %q) = %v, want %v", tc.r, tc.peek, got, tc.want)
		}
	}
}
