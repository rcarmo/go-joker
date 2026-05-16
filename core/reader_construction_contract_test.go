package core

import (
	"io"
	"strconv"
	"strings"
	"testing"
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
