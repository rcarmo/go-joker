package runtime

import "testing"

func TestMakeStringVector(t *testing.T) {
	v := MakeStringVector([]string{"a", "b"})
	if v.Count() != 2 || v.At(0).ToString(false) != "a" || v.At(1).ToString(false) != "b" {
		t.Fatalf("MakeStringVector mismatch: %s", v.ToString(false))
	}
}
