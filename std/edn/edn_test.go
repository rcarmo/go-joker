package edn

import (
	"testing"

	. "github.com/candid82/joker/core"
)

func roundTrip(t *testing.T, src string, want string) {
	t.Helper()
	obj, err := ReadEDNString(src)
	if err != nil {
		t.Fatalf("ReadEDNString(%q): %v", src, err)
	}
	if got := WriteEDNString(obj); got != want {
		t.Fatalf("roundtrip %q = %q, want %q", src, got, want)
	}
}

func TestEDNReadStringPrimitives(t *testing.T) {
	roundTrip(t, "nil", "nil")
	roundTrip(t, "true", "true")
	roundTrip(t, "42", "42")
	roundTrip(t, "3.5", "3.5")
	roundTrip(t, "\"hello\"", "\"hello\"")
	roundTrip(t, ":kw", ":kw")
	roundTrip(t, "sym", "sym")
}

func TestEDNReadStringCollections(t *testing.T) {
	roundTrip(t, "[1 2 :a]", "[1 2 :a]")
	roundTrip(t, "{:a 1, :b [2 3]}", "{:a 1, :b [2 3]}")
	roundTrip(t, "#{1 2 3}", "#{1 2 3}")
}

func TestEDNNumbers(t *testing.T) {
	roundTrip(t, "9223372036854775808N", "9223372036854775808N")
	roundTrip(t, "1/3", "1/3")
	roundTrip(t, "1.25M", "1.25M")
}

func TestEDNDecodeAll(t *testing.T) {
	objs, err := DecodeAllEDN("1 :a [2]")
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 3 || objs[0].(Int).I != 1 || objs[1].ToString(false) != ":a" {
		t.Fatalf("unexpected objects: %#v", objs)
	}
}
