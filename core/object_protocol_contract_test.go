package core

import "testing"

func TestCountedIndexedVectorContract(t *testing.T) {
	items := []Object{MakeInt(1), MakeString("two"), MakeKeyword("three")}
	vectors := []struct {
		name string
		v    Object
	}{
		{name: "array", v: NewArrayVectorFrom(items...)},
		{name: "vector", v: NewVectorFrom(items...)},
		{name: "persistent", v: PersistentVectorFrom(items)},
	}
	for _, tc := range vectors {
		ci, ok := tc.v.(CountedIndexed)
		if !ok {
			t.Fatalf("%s does not implement CountedIndexed", tc.name)
		}
		if ci.Count() != len(items) {
			t.Fatalf("%s Count = %d", tc.name, ci.Count())
		}
		for i, want := range items {
			if !ci.At(i).Equals(want) {
				t.Fatalf("%s At(%d) = %s, want %s", tc.name, i, ci.At(i).ToString(false), want.ToString(false))
			}
		}
		if got := tc.v.ToString(false); got != "[1 two :three]" {
			t.Fatalf("%s ToString = %q", tc.name, got)
		}
	}
	for i := range vectors {
		for j := range vectors {
			if !vectors[i].v.Equals(vectors[j].v) {
				t.Fatalf("%s should equal %s", vectors[i].name, vectors[j].name)
			}
			if vectors[i].v.Hash() != vectors[j].v.Hash() {
				t.Fatalf("%s hash %d != %s hash %d", vectors[i].name, vectors[i].v.Hash(), vectors[j].name, vectors[j].v.Hash())
			}
		}
	}
}

func TestAssociativeMapContract(t *testing.T) {
	entries := []Object{MakeKeyword("a"), MakeInt(1), MakeKeyword("b"), MakeString("two")}
	maps := []struct {
		name string
		m    Map
	}{
		{name: "array", m: EmptyArrayMap().Assoc(entries[0], entries[1]).Assoc(entries[2], entries[3]).(Map)},
		{name: "hash", m: NewHashMap(entries...)},
	}
	for _, tc := range maps {
		if tc.m.Count() != 2 {
			t.Fatalf("%s Count = %d", tc.name, tc.m.Count())
		}
		if found, got := tc.m.Get(MakeKeyword("a")); !found || !got.Equals(MakeInt(1)) {
			t.Fatalf("%s Get(:a) = %v %v", tc.name, found, got)
		}
		updated := tc.m.Assoc(MakeKeyword("a"), MakeInt(10)).(Map)
		if found, got := updated.Get(MakeKeyword("a")); !found || !got.Equals(MakeInt(10)) {
			t.Fatalf("%s updated Get(:a) = %v %v", tc.name, found, got)
		}
		if found, got := tc.m.Get(MakeKeyword("a")); !found || !got.Equals(MakeInt(1)) {
			t.Fatalf("%s Assoc mutated original: %v %v", tc.name, found, got)
		}
	}
	if !maps[0].m.Equals(maps[1].m) || !maps[1].m.Equals(maps[0].m) {
		t.Fatal("array map and hash map should compare equal")
	}
	if maps[0].m.Hash() != maps[1].m.Hash() {
		t.Fatalf("map hash mismatch: array=%d hash=%d", maps[0].m.Hash(), maps[1].m.Hash())
	}
}

func TestSetContract(t *testing.T) {
	set := EmptySet().Conj(MakeInt(1)).Conj(MakeInt(2)).(*MapSet)
	if set.Count() != 2 {
		t.Fatalf("Count = %d, want 2", set.Count())
	}
	if found, got := set.Get(MakeInt(1)); !found || !got.Equals(MakeInt(1)) {
		t.Fatalf("Get(1) = %v %v", found, got)
	}
	if got := set.Call([]Object{MakeInt(2)}); !got.Equals(MakeInt(2)) {
		t.Fatalf("Call(2) = %s", got.ToString(false))
	}
	if got := set.Call([]Object{MakeInt(3)}); got != NIL {
		t.Fatalf("Call(3) = %s, want nil", got.ToString(false))
	}
	removed := set.Disjoin(MakeInt(1)).(*MapSet)
	if found, _ := removed.Get(MakeInt(1)); found {
		t.Fatal("Disjoin result still contains removed value")
	}
	if found, _ := set.Get(MakeInt(1)); !found {
		t.Fatal("Disjoin mutated original set")
	}
	set2 := NewSetFromSeq(NewListFrom(MakeInt(2), MakeInt(1)).Seq())
	if !set.Equals(set2) || set.Hash() != set2.Hash() {
		t.Fatalf("equivalent sets should compare equal with same hash: %s / %s", set.ToString(false), set2.ToString(false))
	}
	meta := EmptyArrayMap().Assoc(MakeKeyword("tag"), MakeString("kept")).(Map)
	withMeta := set.WithMeta(meta).(*MapSet)
	conjMeta := withMeta.Conj(MakeInt(3)).(Meta)
	if found, got := conjMeta.GetMeta().Get(MakeKeyword("tag")); !found || !got.Equals(MakeString("kept")) {
		t.Fatal("set Conj did not preserve metadata")
	}
	disjoinMeta := withMeta.Disjoin(MakeInt(1)).(Meta)
	if found, got := disjoinMeta.GetMeta().Get(MakeKeyword("tag")); !found || !got.Equals(MakeString("kept")) {
		t.Fatal("set Disjoin did not preserve metadata")
	}
}

func TestTransientContract(t *testing.T) {
	vec := NewArrayVectorFrom(MakeInt(1), MakeInt(2))
	tv := ToTransient(vec)
	if _, ok := any(tv).(CountedIndexed); !ok {
		t.Fatal("TransientVector should implement CountedIndexed")
	}
	if tv.Count() != 2 || !tv.At(1).Equals(MakeInt(2)) {
		t.Fatalf("transient vector Count/At mismatch: count=%d at1=%s", tv.Count(), tv.At(1).ToString(false))
	}
	tv.AssocInPlace(MakeInt(1), MakeInt(20)).ConjInPlace(MakeInt(3))
	if tv.Count() != 3 || !tv.At(1).Equals(MakeInt(20)) || !tv.At(2).Equals(MakeInt(3)) {
		t.Fatalf("transient vector mutation mismatch: %s %s %s", tv.At(0).ToString(false), tv.At(1).ToString(false), tv.At(2).ToString(false))
	}
	pv := tv.ToPersistent()
	if pv.Count() != 3 || !pv.At(1).Equals(MakeInt(20)) {
		t.Fatalf("persistent vector round trip mismatch: %s", pv.ToString(false))
	}
	assertPanics(t, "mutating frozen transient vector", func() { tv.ConjInPlace(MakeInt(4)) })

	m := EmptyArrayMap().Assoc(MakeKeyword("a"), MakeInt(1)).Assoc(MakeString("s"), MakeInt(2)).(Map)
	tm := MapToTransient(m)
	tm.AssocInPlace(MakeKeyword("a"), MakeInt(10)).AssocInPlace(MakeString("t"), MakeInt(3))
	if tm.Count() != 3 {
		t.Fatalf("transient map Count = %d, want 3", tm.Count())
	}
	if found, got := tm.Get(MakeKeyword("a")); !found || !got.Equals(MakeInt(10)) {
		t.Fatalf("transient map keyword get = %v %v", found, got)
	}
	if found, got := tm.Get(MakeString("t")); !found || !got.Equals(MakeInt(3)) {
		t.Fatalf("transient map string get = %v %v", found, got)
	}
	pm := tm.ToPersistent().(Map)
	if pm.Count() != 3 {
		t.Fatalf("persistent map Count = %d, want 3", pm.Count())
	}
	if found, got := pm.Get(MakeString("t")); !found || !got.Equals(MakeInt(3)) {
		t.Fatalf("persistent map string get = %v %v", found, got)
	}
	assertPanics(t, "mutating frozen transient map", func() { tm.AssocInPlace(MakeKeyword("z"), MakeInt(0)) })
}

func assertPanics(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s did not panic", name)
		}
	}()
	f()
}

func TestInfoAndMetaContract(t *testing.T) {
	info := &ObjectInfo{Position: Position{startLine: 42}}
	meta := EmptyArrayMap().Assoc(MakeKeyword("doc"), MakeString("sample")).(Map)
	values := []Object{
		NewArrayVectorFrom(MakeInt(1)),
		NewVectorFrom(MakeInt(1)),
		PersistentVectorFrom([]Object{MakeInt(1)}),
	}
	for _, v := range values {
		withInfo := v.WithInfo(info)
		if withInfo.GetInfo() != info {
			t.Fatalf("%T WithInfo did not retain info", v)
		}
		withMeta, ok := withInfo.(Meta)
		if !ok {
			t.Fatalf("%T does not implement Meta after WithInfo", withInfo)
		}
		updated := withMeta.WithMeta(meta).(Meta)
		if found, got := updated.GetMeta().Get(MakeKeyword("doc")); !found || !got.Equals(MakeString("sample")) {
			t.Fatalf("%T WithMeta did not retain metadata", v)
		}
		if originalMeta, ok := v.(Meta); ok && originalMeta.GetMeta() != nil {
			if found, _ := originalMeta.GetMeta().Get(MakeKeyword("doc")); found {
				t.Fatalf("%T WithMeta mutated original metadata", v)
			}
		}
	}
}
