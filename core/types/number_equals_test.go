package types

import "testing"

func TestNumberEqualsDefault(t *testing.T) {
	if !NumberEqualsDefault(MakeInt(1), MakeInt(1)) {
		t.Fatal("same ints should be equal")
	}
	if NumberEqualsDefault(MakeInt(1), MakeString("1")) {
		t.Fatal("number should not equal non-number")
	}
}
