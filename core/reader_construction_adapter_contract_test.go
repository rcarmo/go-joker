package core

import (
	"strings"
	"testing"

	corereader "github.com/rcarmo/go-joker/core/reader"
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
	if !readerConstruction.Double(1.5).Equals(MakeDouble(1.5)) {
		t.Fatal("adapter Double mismatch")
	}
	if c, ok := readerConstruction.Comment(";").(Comment); !ok || c.C != ";" {
		t.Fatalf("adapter Comment mismatch: %#v", c)
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

func TestReaderConstructionAdapterMetadata(t *testing.T) {
	meta, ok := readerConstruction.MetadataFromObject(MakeKeyword("private"))
	if !ok {
		t.Fatal("keyword metadata not accepted")
	}
	if found, got := meta.Get(MakeKeyword("private")); !found || !got.Equals(Boolean{B: true}) {
		t.Fatalf("keyword metadata entry = %v %v", found, got)
	}
	vec := collectionConstruction.ArrayVectorFrom(MakeInt(1))
	withMeta, ok := readerConstruction.WithMeta(vec, meta)
	if !ok || withMeta.(Meta).GetMeta() == nil {
		t.Fatalf("WithMeta = %T %v", withMeta, ok)
	}
	if _, ok := readerConstruction.MetadataFromObject(MakeInt(1)); ok {
		t.Fatal("integer metadata accepted")
	}
	if _, ok := readerConstruction.WithMeta(MakeInt(1), meta); ok {
		t.Fatal("metadata applied to int")
	}
}

func TestReaderConstructionAdapterNumberFromToken(t *testing.T) {
	r := readerConstruction.NewReader(strings.NewReader("42"), "<adapter-contract>")
	pushPos(r)
	_ = r.Get()
	n := readerConstruction.NumberFromToken(r, corereader.NumberToken{Kind: corereader.NumberTokenInt, Original: "42", Digits: "42", Base: 10})
	if !n.Equals(MakeInt(42)) {
		t.Fatalf("adapter NumberFromToken = %s, want 42", n.ToString(false))
	}
}

func TestReaderConstructionAdapterCollectionObjects(t *testing.T) {
	list := readerConstruction.ListFrom([]Object{MakeInt(1), MakeInt(2)}).(Seq)
	if SeqCount(list) != 2 || !list.First().Equals(MakeInt(1)) || !Second(list).Equals(MakeInt(2)) {
		t.Fatalf("adapter ListFrom mismatch: %s", list.ToString(false))
	}
	vec := readerConstruction.VectorFrom([]Object{MakeKeyword("a"), MakeKeyword("b")}).(CountedIndexed)
	if vec.Count() != 2 || !vec.At(0).Equals(MakeKeyword("a")) || !vec.At(1).Equals(MakeKeyword("b")) {
		t.Fatalf("adapter VectorFrom mismatch: %s", vec.(Object).ToString(false))
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
