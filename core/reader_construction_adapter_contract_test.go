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
