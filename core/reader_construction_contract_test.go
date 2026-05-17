package core

import (
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"testing"

	corereader "github.com/rcarmo/go-joker/core/reader"
)

func readOneForContract(t *testing.T, src string) Object {
	t.Helper()
	r := NewReader(strings.NewReader(src), "<reader-contract>")
	obj, err := TryRead(r)
	if err != nil {
		t.Fatalf("TryRead(%q): %v", src, err)
	}
	if _, err := TryRead(r); err != io.EOF {
		t.Fatalf("TryRead(%q) should have exactly one form, next err=%v", src, err)
	}
	return obj
}

func requireReadErrorForContract(t *testing.T, src string) {
	t.Helper()
	r := NewReader(strings.NewReader(src), "<reader-contract>")
	if obj, err := TryRead(r); err == nil {
		t.Fatalf("TryRead(%q) = %s, want read error", src, obj.ToString(false))
	}
}

func TestReaderConstructionAdapterReaderSurface(t *testing.T) {
	adapter := ReaderConstructionAdapter{}
	reader := adapter.NewReader(strings.NewReader("[1 2]"), "<adapter>")
	obj, err := adapter.TryRead(reader)
	if err != nil {
		t.Fatalf("TryRead via adapter: %v", err)
	}
	vec := obj.(CountedIndexed)
	if vec.Count() != 2 || !vec.At(0).Equals(MakeInt(1)) || obj.GetInfo().Filename() != "<adapter>" {
		t.Fatalf("adapter reader result mismatch: %s info=%#v", obj.ToString(false), obj.GetInfo())
	}
	if _, err := adapter.TryRead(reader); err != io.EOF {
		t.Fatalf("adapter TryRead should reach EOF, got %v", err)
	}
}

func TestReaderConstructionAdapterExpressionSurface(t *testing.T) {
	adapter := ReaderConstructionAdapter{}
	obj := MakeString("literal")
	lit := adapter.LiteralExpr(obj)
	if lit.obj != obj || lit.isSurrogate {
		t.Fatalf("LiteralExpr mismatch: %#v", lit)
	}
	surrogate := adapter.SurrogateExpr(obj)
	if surrogate.obj != obj || !surrogate.isSurrogate {
		t.Fatalf("SurrogateExpr mismatch: %#v", surrogate)
	}
	pos := Position{startLine: 1, startColumn: 2, endLine: 1, endColumn: 3}
	vec := adapter.VectorExpr([]Expr{lit}, pos)
	if len(vec.v) != 1 || vec.Position != pos {
		t.Fatalf("VectorExpr mismatch: %#v", vec)
	}
	m := adapter.MapExpr(2, pos)
	if len(m.keys) != 2 || len(m.values) != 2 || m.Position != pos {
		t.Fatalf("MapExpr mismatch: %#v", m)
	}
	set := adapter.SetExpr(3, pos)
	if len(set.elements) != 3 || set.Position != pos {
		t.Fatalf("SetExpr mismatch: %#v", set)
	}
	setFrom := adapter.SetExprFrom([]Expr{lit}, pos)
	if len(setFrom.elements) != 1 || setFrom.elements[0] != lit || setFrom.Position != pos {
		t.Fatalf("SetExprFrom mismatch: %#v", setFrom)
	}
	mapFrom := adapter.MapExprFrom([]Expr{lit}, []Expr{surrogate}, pos)
	if len(mapFrom.keys) != 1 || mapFrom.keys[0] != lit || len(mapFrom.values) != 1 || mapFrom.values[0] != surrogate || mapFrom.Position != pos {
		t.Fatalf("MapExprFrom mismatch: %#v", mapFrom)
	}
}

func TestReadIntegerUsesNativeIntRange(t *testing.T) {
	got := readOneForContract(t, "1099511627776")
	if strconv.IntSize == 32 {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("32-bit integer literal type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("64-bit integer literal type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestReaderConstructionContractPrimitivesAndCollections(t *testing.T) {
	cases := []struct {
		src  string
		want Object
	}{
		{`nil`, NIL},
		{`true`, Boolean{B: true}},
		{`42`, MakeInt(42)},
		{`"hi"`, MakeString("hi")},
		{`:kw`, MakeKeyword("kw")},
		{`sym`, MakeSymbol("sym")},
	}
	for _, tc := range cases {
		got := readOneForContract(t, tc.src)
		if !got.Equals(tc.want) {
			t.Fatalf("read %s = %s (%T), want %s (%T)", tc.src, got.ToString(false), got, tc.want.ToString(false), tc.want)
		}
	}

	vecObj := readOneForContract(t, `[1 :two "three"]`)
	if vecObj.GetInfo() == nil || vecObj.GetInfo().Filename() != "<reader-contract>" {
		t.Fatalf("vector did not retain source info: %#v", vecObj.GetInfo())
	}
	vec := vecObj.(CountedIndexed)
	if vec.Count() != 3 || !vec.At(0).Equals(MakeInt(1)) || !vec.At(1).Equals(MakeKeyword("two")) || !vec.At(2).Equals(MakeString("three")) {
		t.Fatalf("vector construction mismatch: %s", vec.(Object).ToString(false))
	}
	m := readOneForContract(t, `{:a 1 "b" 2}`).(Map)
	if m.Count() != 2 {
		t.Fatalf("map count = %d, want 2", m.Count())
	}
	if ok, got := m.Get(MakeKeyword("a")); !ok || !got.Equals(MakeInt(1)) {
		t.Fatalf("map keyword entry = %v %v", ok, got)
	}
	set := readOneForContract(t, `#{3 1 2}`).(*MapSet)
	if set.Count() != 3 {
		t.Fatalf("set count = %d, want 3", set.Count())
	}
	namespaced := readOneForContract(t, `#:contract{:a 1 :b 2}`).(Map)
	if found, got := namespaced.Get(MakeKeyword("contract/a")); !found || !got.Equals(MakeInt(1)) {
		t.Fatalf("namespaced map keyword entry = %v %v", found, got)
	}
	list := readOneForContract(t, `(1 2 3)`).(Seq)
	if SeqCount(list) != 3 || !list.First().Equals(MakeInt(1)) || !Third(list).Equals(MakeInt(3)) {
		t.Fatalf("list construction mismatch: %s", list.ToString(false))
	}

	requireReadErrorForContract(t, `{:a 1 :a 2}`)
	requireReadErrorForContract(t, `#{1 1}`)
	requireReadErrorForContract(t, `{:a 1 :b}`)
	requireReadErrorForContract(t, `#:#?@(:clj [foo]){:a 1}`)
}

func TestReaderConstructionContractRejectsInvalidArgLiteral(t *testing.T) {
	if _, err := TryRead(NewReader(strings.NewReader(`#(%0)`), "<reader-contract>")); err == nil {
		t.Fatal("expected invalid %0 arg literal to fail")
	}
}

func TestReaderConstructionContractMetadataTaggedReadersAndConditionals(t *testing.T) {
	metaObj := readOneForContract(t, `^:private [1 2]`)
	meta, ok := metaObj.(Meta)
	if !ok || meta.GetMeta() == nil {
		t.Fatalf("metadata reader should produce Meta object: %T", metaObj)
	}
	if found, got := meta.GetMeta().Get(MakeKeyword("private")); !found || !got.Equals(Boolean{B: true}) {
		t.Fatalf("metadata did not contain :private true: %v %v", found, got)
	}
	if metaObj.GetInfo() == nil || metaObj.GetInfo().Filename() != "<reader-contract>" {
		t.Fatalf("metadata form did not preserve source info: %#v", metaObj.GetInfo())
	}

	dataReadersVar := GLOBAL_ENV.CoreNamespace.Resolve("*data-readers*")
	if dataReadersVar == nil {
		t.Fatal("*data-readers* var not found")
	}
	oldDataReaders := dataReadersVar.Value
	readers := EmptyArrayMap()
	readers.Add(MakeSymbol("contract/direct"), Proc{Name: "readerContractDirect", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		return NewArrayVectorFrom(MakeKeyword("direct"), args[0])
	}})
	dataReadersVar.Value = readers
	defer func() { dataReadersVar.Value = oldDataReaders }()

	direct := readOneForContract(t, `#contract/direct 7`).(CountedIndexed)
	if direct.Count() != 2 || !direct.At(0).Equals(MakeKeyword("direct")) || !direct.At(1).Equals(MakeInt(7)) {
		t.Fatalf("direct tagged reader mismatch: %s", direct.(Object).ToString(false))
	}

	fallbackVar := GLOBAL_ENV.CoreNamespace.Resolve("*default-data-reader-fn*")
	if fallbackVar == nil {
		t.Fatal("*default-data-reader-fn* var not found")
	}
	oldFallback := fallbackVar.Value
	fallbackVar.Value = Proc{Name: "readerContractFallback", Fn: func(args []Object) Object {
		CheckArity(args, 2, 2)
		return NewArrayVectorFrom(args[0], args[1])
	}}
	defer func() { fallbackVar.Value = oldFallback }()

	tagged := readOneForContract(t, `#contract/tag {:x 1}`).(CountedIndexed)
	if tagged.Count() != 2 || !tagged.At(0).Equals(MakeSymbol("contract/tag")) {
		t.Fatalf("tagged fallback mismatch: %s", tagged.(Object).ToString(false))
	}
	payload, ok := tagged.At(1).(Map)
	if !ok {
		t.Fatalf("tagged fallback payload = %T, want Map", tagged.At(1))
	}
	if found, got := payload.Get(MakeKeyword("x")); !found || !got.Equals(MakeInt(1)) {
		t.Fatalf("tagged fallback payload entry = %v %v", found, got)
	}

	selected := readOneForContract(t, `#?(:missing :no :joker [1 2])`)
	selectedVec, ok := selected.(CountedIndexed)
	if !ok || selectedVec.Count() != 2 || !selectedVec.At(0).Equals(MakeInt(1)) || !selectedVec.At(1).Equals(MakeInt(2)) {
		t.Fatalf("reader conditional selected wrong form: %s", selected.ToString(false))
	}
	spliced := readOneForContract(t, `(#?@(:missing [:no] :joker [1 2]) 3)`).(Seq)
	if SeqCount(spliced) != 3 || !spliced.First().Equals(MakeInt(1)) || !Second(spliced).Equals(MakeInt(2)) || !Third(spliced).Equals(MakeInt(3)) {
		t.Fatalf("reader conditional splice mismatch: %s", spliced.ToString(false))
	}
}

func TestReadConditionalSpliceEmptyInList(t *testing.T) {
	reader := NewReader(strings.NewReader("(do #?@(:definitely-nope [1 2]) 3)"), "<test>")
	obj, err := TryRead(reader)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	lst, ok := obj.(*List)
	if !ok {
		t.Fatalf("expected list, got %T", obj)
	}
	if got := lst.Count(); got != 2 {
		t.Fatalf("expected 2 elements (do, 3), got %d", got)
	}
}

func TestReadConditionalNestedSpliceNoRuntimePanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected runtime panic: %v", r)
		}
	}()

	reader := NewReader(strings.NewReader("#?(:x #?@(:x [1 2]))"), "<test>")
	_, err := TryRead(reader)
	if err == nil {
		t.Fatal("expected read error for invalid nested splice")
	}
	if !strings.Contains(err.Error(), "Read error") {
		t.Fatalf("expected read error, got: %v", err)
	}
}

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
	if !readerConstruction.Int(7).Equals(MakeInt(7)) {
		t.Fatal("adapter Int mismatch")
	}
	if !readerConstruction.Double(1.5).Equals(MakeDouble(1.5)) {
		t.Fatal("adapter Double mismatch")
	}
	if c, ok := readerConstruction.Comment(";").(Comment); !ok || c.C != ";" {
		t.Fatalf("adapter Comment mismatch: %#v", c)
	}
	rxObj, ok := readerConstruction.Regex(regexp.MustCompile("x+")).(*Regex)
	if !ok || rxObj.R == nil || !rxObj.R.MatchString("xxx") {
		t.Fatalf("adapter Regex mismatch: %#v", rxObj)
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

func TestReaderConstructionAdapterSetLiteral(t *testing.T) {
	r := readerConstruction.NewReader(strings.NewReader("#{}"), "<adapter-contract>")
	pushPos(r)
	set := readerConstruction.SetLiteral(r, []Object{MakeInt(1), MakeInt(2)}).(*MapSet)
	if set.Count() != 2 {
		t.Fatalf("SetLiteral count = %d, want 2", set.Count())
	}
	obj := readerConstruction.ReadObject(r, set)
	if obj.GetInfo() == nil || obj.GetInfo().Filename() != "<adapter-contract>" {
		t.Fatalf("SetLiteral read object info = %#v", obj.GetInfo())
	}
}

func TestReaderConstructionAdapterMapLiteral(t *testing.T) {
	r := readerConstruction.NewReader(strings.NewReader("{}"), "<adapter-contract>")
	pushPos(r)
	m := readerConstruction.MapLiteral(r, []Object{MakeKeyword("a"), MakeInt(1)}, "").(Map)
	if found, got := m.Get(MakeKeyword("a")); !found || !got.Equals(MakeInt(1)) {
		t.Fatalf("MapLiteral entry = %v %v", found, got)
	}
	obj := readerConstruction.ReadObject(r, m)
	if obj.GetInfo() == nil || obj.GetInfo().Filename() != "<adapter-contract>" {
		t.Fatalf("MapLiteral read object info = %#v", obj.GetInfo())
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
	skip := readerConstruction.SkipRedundantDoMeta()
	if found, got := skip.Get(MakeKeyword("skip-redundant-do")); !found || !got.Equals(Boolean{B: true}) {
		t.Fatalf("SkipRedundantDoMeta entry = %v %v", found, got)
	}
}

func TestReaderConstructionAdapterNumericObjects(t *testing.T) {
	bi := readerConstruction.BigInt(big.NewInt(42), "42")
	if bi.GetType() != TYPE.BigInt || bi.ToString(false) != "42N" {
		t.Fatalf("adapter BigInt = %s type=%s", bi.ToString(false), bi.GetType().ToString(false))
	}
	bf, ok := readerConstruction.BigFloatFromString("1.25", "1.25M")
	if !ok || bf.GetType() != TYPE.BigFloat {
		t.Fatalf("adapter BigFloat = %v %T", ok, bf)
	}
	r := readerConstruction.RatioOrInt("2/4", big.NewRat(2, 4))
	if !r.Equals(MakeInt(0)) && r.GetType() != TYPE.Ratio && r.GetType() != TYPE.Int {
		t.Fatalf("adapter RatioOrInt unexpected: %s type=%s", r.ToString(false), r.GetType().ToString(false))
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
	persistent := readerConstruction.PersistentVectorFromSeq(vec.(Seqable).Seq()).(CountedIndexed)
	if persistent.Count() != 2 || !persistent.At(0).Equals(MakeKeyword("a")) || !persistent.At(1).Equals(MakeKeyword("b")) {
		t.Fatalf("adapter PersistentVectorFromSeq mismatch: %s", persistent.(Object).ToString(false))
	}
	if readerConstruction.VectorFrom(nil).(Counted).Count() != 0 {
		t.Fatal("adapter empty VectorFrom not empty")
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
