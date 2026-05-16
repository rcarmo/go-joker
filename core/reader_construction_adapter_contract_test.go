package core

import (
	"strings"
	"testing"
)

func TestReaderConstructionAdapterReadObjectAndError(t *testing.T) {
	r := readerConstruction.NewReader(strings.NewReader("x"), "<adapter-contract>")
	pushPos(r)
	_ = r.Get()
	obj := readerConstruction.ReadObject(r, MakeSymbol("x"))
	info := obj.GetInfo()
	if info == nil || info.Filename() != "<adapter-contract>" || info.startLine != 1 || info.startColumn != 0 || info.endLine != 1 || info.endColumn != 1 {
		t.Fatalf("adapter ReadObject info = %#v", info)
	}
	err := readerConstruction.ReadError(r, "boom")
	if !strings.Contains(err.Error(), "<adapter-contract>:1:1") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("adapter ReadError = %v", err)
	}
}

func TestReaderConstructionAdapterScalarObjects(t *testing.T) {
	if !readerConstruction.Nil().Equals(NIL) {
		t.Fatal("adapter Nil mismatch")
	}
	if !readerConstruction.Bool(true).Equals(Boolean{B: true}) || !readerConstruction.Bool(false).Equals(Boolean{B: false}) {
		t.Fatal("adapter Bool mismatch")
	}
	if !readerConstruction.Char('x').Equals(Char{Ch: 'x'}) {
		t.Fatal("adapter Char mismatch")
	}
	if !readerConstruction.String("x").Equals(MakeString("x")) {
		t.Fatal("adapter String mismatch")
	}
	if !readerConstruction.Symbol("x").Equals(MakeSymbol("x")) {
		t.Fatal("adapter Symbol mismatch")
	}
	if !readerConstruction.Keyword("x").Equals(MakeKeyword("x")) {
		t.Fatal("adapter Keyword mismatch")
	}
}

func TestReaderConstructionAdapterDeriveReadObject(t *testing.T) {
	r := readerConstruction.NewReader(strings.NewReader("x"), "<adapter-contract>")
	pushPos(r)
	_ = r.Get()
	base := readerConstruction.ReadObject(r, MakeSymbol("x"))
	derived := readerConstruction.DeriveReadObject(base, MakeKeyword("x"))
	if derived.GetInfo() == nil || derived.GetInfo().Filename() != "<adapter-contract>" {
		t.Fatalf("derived info = %#v", derived.GetInfo())
	}
}
