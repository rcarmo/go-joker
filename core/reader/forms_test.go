package reader

import "testing"

func TestMapFormHelpers(t *testing.T) {
	if !ShouldAppendMapCommentSurrogate(true, true) {
		t.Fatal("format comment should append surrogate")
	}
	if ShouldAppendMapCommentSurrogate(false, true) || ShouldAppendMapCommentSurrogate(true, false) {
		t.Fatal("surrogate helper true outside format comments")
	}
	if !HasEvenFormCount(0) || !HasEvenFormCount(2) || HasEvenFormCount(3) {
		t.Fatal("unexpected even form count result")
	}
	if !IsBareArgLiteral(' ') || !IsBareArgLiteral(')') || IsBareArgLiteral('1') {
		t.Fatal("unexpected bare arg literal classification")
	}
}
