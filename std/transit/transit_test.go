package transit

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestTransitRoundTripMap(t *testing.T) {
	m := EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString("joker"))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "n"), coretypes.MakeInt(42))
	encoded := writeTransit(m).(coretypes.String).S
	if !strings.Contains(encoded, `"^ "`) || !strings.Contains(encoded, `"~:name"`) {
		t.Fatalf("unexpected transit map encoding: %s", encoded)
	}
	decoded := readTransit(coretypes.MakeString(encoded)).(coretypes.Map)
	if ok, v := decoded.Get(coretypes.MakeKeyword(STRINGS.Intern, "name")); !ok || v.ToString(false) != "joker" {
		t.Fatalf("name roundtrip failed: %v", v)
	}
	if ok, v := decoded.Get(coretypes.MakeKeyword(STRINGS.Intern, "n")); !ok || v.(coretypes.Int).I != 42 {
		t.Fatalf("n roundtrip failed: %v", v)
	}
}

func TestTransitCacheRefs(t *testing.T) {
	v := NewVectorFrom(coretypes.MakeKeyword(STRINGS.Intern, "repeat-key"), coretypes.MakeKeyword(STRINGS.Intern, "repeat-key"))
	encoded := writeTransit(v).(coretypes.String).S
	if !strings.Contains(encoded, `"^0"`) {
		t.Fatalf("expected cache ref in encoding: %s", encoded)
	}
	decoded := readTransit(coretypes.MakeString(encoded)).(coretypes.CountedIndexed)
	if decoded.At(0).ToString(false) != ":repeat-key" || decoded.At(1).ToString(false) != ":repeat-key" {
		t.Fatalf("cache ref roundtrip failed: %s %s", decoded.At(0).ToString(false), decoded.At(1).ToString(false))
	}
	verbose := writeTransitVerbose(v).(coretypes.String).S
	if strings.Contains(verbose, `"^0"`) {
		t.Fatalf("verbose writer should not use cache refs: %s", verbose)
	}
}

func TestTransitTaggedSetListQuoteCMap(t *testing.T) {
	set := readTransit(coretypes.MakeString(`["~#set",[1,"~:a"]]`)).(*MapSet)
	if ok, _ := set.Get(coretypes.MakeInt(1)); !ok {
		t.Fatalf("set missing int: %s", set.ToString(false))
	}
	lst := readTransit(coretypes.MakeString(`["~#list",[1,2]]`))
	if lst.ToString(false) != "(1 2)" {
		t.Fatalf("list tag mismatch: %s", lst.ToString(false))
	}
	quoted := readTransit(coretypes.MakeString(`["~#'","~:quoted"]`))
	if quoted.ToString(false) != ":quoted" {
		t.Fatalf("quote tag mismatch: %s", quoted.ToString(false))
	}
	cmap := readTransit(coretypes.MakeString(`["~#cmap",["~:k",1,"~$sym",2]]`)).(coretypes.Map)
	if ok, v := cmap.Get(coretypes.MakeKeyword(STRINGS.Intern, "k")); !ok || v.(coretypes.Int).I != 1 {
		t.Fatalf("cmap keyword mismatch: %s", cmap.ToString(false))
	}
}

func TestTransitCMapRejectsOddEntries(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("odd cmap entries did not panic")
		}
	}()
	_ = readTransit(coretypes.MakeString(`["~#cmap",["~:k",1,"~:dangling"]]`))
}

func TestTransitBigNumbersAndRatio(t *testing.T) {
	big := readTransit(coretypes.MakeString(`"~i9223372036854775808"`))
	if big.GetType() != TYPE.BigInt || big.ToString(false) != "9223372036854775808N" {
		t.Fatalf("big int mismatch: %T %s", big, big.ToString(false))
	}
	dec := readTransit(coretypes.MakeString(`"~f0.125"`))
	if dec.GetType() != TYPE.BigFloat || dec.ToString(false) != "0.125M" {
		t.Fatalf("big float mismatch: %T %s", dec, dec.ToString(false))
	}
	ratio := readTransit(coretypes.MakeString(`"~r2/3"`))
	if ratio.GetType() != TYPE.Ratio || ratio.ToString(false) != "2/3" {
		t.Fatalf("ratio mismatch: %T %s", ratio, ratio.ToString(false))
	}
}

func TestTransitEscapedStringsAndSymbols(t *testing.T) {
	v := NewVectorFrom(coretypes.MakeString("~literal"), coretypes.MakeKeyword(STRINGS.Intern, "k"), coretypes.MakeSymbol(STRINGS.Intern, "sym"))
	encoded := writeTransit(v).(coretypes.String).S
	decoded := readTransit(coretypes.MakeString(encoded)).(coretypes.CountedIndexed)
	if decoded.At(0).ToString(false) != "~literal" {
		t.Fatalf("escaped string failed: %s", decoded.At(0).ToString(false))
	}
	if decoded.At(1).ToString(false) != ":k" || decoded.At(2).ToString(false) != "sym" {
		t.Fatalf("tagged value roundtrip failed: %s %s", decoded.At(1).ToString(false), decoded.At(2).ToString(false))
	}
}
