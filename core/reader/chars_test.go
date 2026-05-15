package reader

import "testing"

func TestReaderCharacterClassification(t *testing.T) {
	for _, r := range []rune{' ', '\t', '\n', '\r', ','} {
		if !IsWhitespace(r) {
			t.Fatalf("%q should be whitespace", r)
		}
	}
	if IsWhitespace('x') {
		t.Fatal("x should not be whitespace")
	}
	for _, r := range []rune{'"', ';', '@', '^', '`', '~', '(', ')', '[', ']', '{', '}', '\\'} {
		if !IsTerminatingMacro(r) {
			t.Fatalf("%q should be terminating macro", r)
		}
	}
	for _, r := range []rune{'a', ',', ' ', EOF} {
		if IsTerminatingMacro(r) {
			t.Fatalf("%q should not be terminating macro", r)
		}
	}
	for _, r := range []rune{')', ']', '}'} {
		if !IsClosingDelimiter(r) {
			t.Fatalf("%q should be closing delimiter", r)
		}
	}
	if IsClosingDelimiter('(') {
		t.Fatal("opening delimiter should not be closing delimiter")
	}
	if !IsCommentStart(';', 0) || !IsCommentStart('#', '!') || IsCommentStart('#', '_') {
		t.Fatal("unexpected comment start classification")
	}
	for _, r := range []rune{'(', ')', '[', ']', '{', '}', '"', ';', EOF, '\\', ' '} {
		if !IsDelimiter(r) {
			t.Fatalf("%q should be delimiter", r)
		}
	}
	if IsDelimiter('x') {
		t.Fatal("x should not be delimiter")
	}
	if IsJavaSpace('\u00a0') {
		t.Fatal("non-breaking space should follow Java non-space behavior")
	}
}
