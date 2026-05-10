package transit

import (
	"strings"
	"testing"

	. "github.com/candid82/joker/core"
)

func TestTransitRoundTripMap(t *testing.T) {
	m := EmptyArrayMap()
	m.Add(MakeKeyword("name"), MakeString("joker"))
	m.Add(MakeKeyword("n"), MakeInt(42))
	encoded := writeTransit(m).(String).S
	if !strings.Contains(encoded, `"^ "`) || !strings.Contains(encoded, `"~:name"`) {
		t.Fatalf("unexpected transit map encoding: %s", encoded)
	}
	decoded := readTransit(MakeString(encoded)).(Map)
	if ok, v := decoded.Get(MakeKeyword("name")); !ok || v.ToString(false) != "joker" {
		t.Fatalf("name roundtrip failed: %v", v)
	}
	if ok, v := decoded.Get(MakeKeyword("n")); !ok || v.(Int).I != 42 {
		t.Fatalf("n roundtrip failed: %v", v)
	}
}

func TestTransitEscapedStringsAndSymbols(t *testing.T) {
	v := NewVectorFrom(MakeString("~literal"), MakeKeyword("k"), MakeSymbol("sym"))
	encoded := writeTransit(v).(String).S
	decoded := readTransit(MakeString(encoded)).(CountedIndexed)
	if decoded.At(0).ToString(false) != "~literal" {
		t.Fatalf("escaped string failed: %s", decoded.At(0).ToString(false))
	}
	if decoded.At(1).ToString(false) != ":k" || decoded.At(2).ToString(false) != "sym" {
		t.Fatalf("tagged value roundtrip failed: %s %s", decoded.At(1).ToString(false), decoded.At(2).ToString(false))
	}
}
