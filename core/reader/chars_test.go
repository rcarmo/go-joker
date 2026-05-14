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
