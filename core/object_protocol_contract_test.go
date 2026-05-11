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

func TestInfoAndMetaCopyOnWriteContract(t *testing.T) {
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
				t.Fatalf("%T WithMeta mutated original", v)
			}
		}
	}
}
