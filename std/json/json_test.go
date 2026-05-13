package json

import (
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func expectJSONPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

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

func TestJSONSeqSurfacesDecodeErrors(t *testing.T) {
	seq := jsonSeqOpts(MakeString(`{"ok":1}
{"bad"`), nil).(Seq)
	if seq.First() == nil {
		t.Fatal("expected first json object")
	}
	expectJSONPanic(t, func() { seq.Rest().First() })
}
