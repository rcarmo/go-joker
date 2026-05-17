package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCollectionConstructionAdapterVectors(t *testing.T) {
	adapter := CollectionConstructionAdapter{}
	items := []Object{MakeInt(1), MakeString("two")}

	vector := adapter.VectorFrom(items...)
	if vector.Count() != 2 || !vector.At(1).Equals(items[1]) {
		t.Fatalf("VectorFrom mismatch: %s", vector.ToString(false))
	}
	fromSeq := adapter.VectorFromSeq(NewListFrom(items...).Seq())
	if !fromSeq.Equals(vector) {
		t.Fatalf("VectorFromSeq = %s, want %s", fromSeq.ToString(false), vector.ToString(false))
	}
	array := adapter.ArrayVectorFrom(items...)
	if array.Count() != 2 || !array.At(0).Equals(items[0]) {
		t.Fatalf("ArrayVectorFrom mismatch: %s", array.ToString(false))
	}
	if adapter.EmptyVector().Count() != 0 || adapter.EmptyArrayVector().Count() != 0 {
		t.Fatal("empty vector constructors should return empty collections")
	}
}

func TestCollectionConstructionAdapterMapsAndSets(t *testing.T) {
	adapter := CollectionConstructionAdapter{}
	key := MakeKeyword("k")
	value := MakeInt(42)

	arrayMap := adapter.EmptyArrayMap().Assoc(key, value).(Map)
	hashMap := adapter.HashMapFrom(key, value)
	if !arrayMap.Equals(hashMap) || arrayMap.Hash() != hashMap.Hash() {
		t.Fatalf("map constructors should build equivalent maps: %s / %s", arrayMap.ToString(false), hashMap.ToString(false))
	}
	if found, got := hashMap.Get(key); !found || !got.Equals(value) {
		t.Fatalf("HashMapFrom lookup = %v %v", found, got)
	}

	set := adapter.EmptySet().Conj(MakeInt(1)).Conj(MakeInt(2)).(*MapSet)
	fromSeq := adapter.SetFromSeq(NewListFrom(MakeInt(2), MakeInt(1)).Seq())
	if !set.Equals(fromSeq) || set.Hash() != fromSeq.Hash() {
		t.Fatalf("set constructors should build equivalent sets: %s / %s", set.ToString(false), fromSeq.ToString(false))
	}
}

func TestCollectionConstructionCallSitesUseAdapter(t *testing.T) {
	direct := regexp.MustCompile(`(^|[^.])\b(EmptyVector|NewVectorFrom|NewVectorFromSeq|EmptyArrayVector|NewArrayVectorFrom|EmptyArrayMap|NewHashMap|EmptySet|NewSetFromSeq)\(`)
	allowed := map[string]bool{
		"array_map.go":                        true,
		"array_vector.go":                     true,
		"collection_construction.go":          true,
		"hash_map.go":                         true,
		"object.go":                           true,
		"persistent_vector.go":                true,
		"set.go":                              true,
		"vector.go":                           true,
		"construction_boundary_guard_test.go": true,
	}
	assertNoDirectConstructionOutside(t, direct, allowed)
}

func TestReaderConstructionCallSitesUseAdapter(t *testing.T) {
	direct := regexp.MustCompile(`(^|[^.])\b(NewReader|TryRead|Read|NewLiteralExpr|NewSurrogateExpr)\(|&(VectorExpr|MapExpr|SetExpr)\b`)
	allowed := map[string]bool{
		"parse.go":                            true, // owns parser expression constructor implementations.
		"read.go":                             true, // owns reader read-loop implementations.
		"reader.go":                           true, // owns Reader constructor implementation.
		"reader_construction.go":              true,
		"construction_boundary_guard_test.go": true,
	}
	assertNoDirectConstructionOutside(t, direct, allowed)
}

func assertNoDirectConstructionOutside(t *testing.T, direct *regexp.Regexp, allowed map[string]bool) {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		base := filepath.Base(file)
		if strings.HasSuffix(base, "_test.go") && base != "construction_boundary_guard_test.go" {
			continue
		}
		if allowed[base] {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for lineNo, line := range strings.Split(string(data), "\n") {
			if direct.MatchString(line) {
				t.Fatalf("%s:%d uses direct construction instead of adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
		}
	}
}
