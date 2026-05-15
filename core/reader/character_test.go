package reader

import "testing"

func TestClassifyCharacterLiteral(t *testing.T) {
	if got := ClassifyCharacterLiteral('u', '0'); got != CharacterLiteralUnicode {
		t.Fatalf("ClassifyCharacterLiteral unicode = %v", got)
	}
	if got := ClassifyCharacterLiteral('o', '1'); got != CharacterLiteralOctal {
		t.Fatalf("ClassifyCharacterLiteral octal = %v", got)
	}
	if got := ClassifyCharacterLiteral('u', ' '); got != CharacterLiteralPlain {
		t.Fatalf("ClassifyCharacterLiteral delimited unicode = %v", got)
	}
	if got := ClassifyCharacterLiteral('x', 'y'); got != CharacterLiteralPlain {
		t.Fatalf("ClassifyCharacterLiteral plain = %v", got)
	}
}

func TestNamedCharacter(t *testing.T) {
	tests := []struct {
		first  rune
		peek   rune
		ending string
		value  rune
	}{
		{first: 's', peek: 'p', ending: "pace", value: ' '},
		{first: 'n', peek: 'e', ending: "ewline", value: '\n'},
		{first: 't', peek: 'a', ending: "ab", value: '\t'},
		{first: 'f', peek: 'o', ending: "ormfeed", value: '\f'},
		{first: 'b', peek: 'a', ending: "ackspace", value: '\b'},
		{first: 'r', peek: 'e', ending: "eturn", value: '\r'},
	}
	for _, tt := range tests {
		ending, value, ok := NamedCharacter(tt.first, tt.peek)
		if !ok || ending != tt.ending || value != tt.value {
			t.Fatalf("NamedCharacter(%q,%q) = %q/%q/%v, want %q/%q/true", tt.first, tt.peek, ending, value, ok, tt.ending, tt.value)
		}
	}
	if _, _, ok := NamedCharacter('x', 'y'); ok {
		t.Fatal("NamedCharacter unknown returned ok")
	}
}
