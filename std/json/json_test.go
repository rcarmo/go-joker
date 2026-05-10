package json

import (
	"testing"

	. "github.com/candid82/joker/core"
)

func TestJSONReadStringKeywordizeAndFromObject(t *testing.T) {
	opts := EmptyArrayMap()
	opts.Add(MakeKeyword("keywords?"), Boolean{B: true})
	obj := readString(`{"a":1,"b":[true,"x"]}`, opts).(Map)
	ok, a := obj.Get(MakeKeyword("a"))
	if !ok || a.(Int).I != 1 {
		t.Fatalf("keywordized a mismatch: %v", a)
	}
	m := EmptyArrayMap()
	m.Add(MakeKeyword("k"), MakeString("v"))
	encoded := fromObject(m).(map[string]interface{})
	if encoded["k"] != "v" {
		t.Fatalf("fromObject mismatch: %#v", encoded)
	}
}
