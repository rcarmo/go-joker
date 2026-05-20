package json

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

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
	opts := corecollections.EmptyArrayMap()
	opts.Add(coretypes.MakeKeyword(STRINGS.Intern, "keywords?"), coretypes.Boolean{B: true})
	obj := readString(`{"a":1,"b":[true,"x"]}`, opts).(coretypes.Map)
	ok, a := obj.Get(coretypes.MakeKeyword(STRINGS.Intern, "a"))
	if !ok || a.(coretypes.Int).I != 1 {
		t.Fatalf("keywordized a mismatch: %v", a)
	}
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "k"), coretypes.MakeString("v"))
	encoded := fromObject(m).(map[string]interface{})
	if encoded["k"] != "v" {
		t.Fatalf("fromObject mismatch: %#v", encoded)
	}
}

func TestJSONSeqSurfacesDecodeErrors(t *testing.T) {
	seq := jsonSeqOpts(coretypes.MakeString(`{"ok":1}
{"bad"`), nil).(coretypes.Seq)
	if seq.First() == nil {
		t.Fatal("expected first json object")
	}
	expectJSONPanic(t, func() { seq.Rest().First() })
}
