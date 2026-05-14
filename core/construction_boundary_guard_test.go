package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

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
