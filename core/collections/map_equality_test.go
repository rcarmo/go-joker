package collections

import "testing"

func TestEqualMaps(t *testing.T) {
	left := []Pair[string, int]{{Key: "a", Value: 1}, {Key: "b", Value: 2}}
	right := map[string]int{"a": 1, "b": 2}
	iter := func(yield func(Pair[string, int]) bool) {
		for _, p := range left {
			if !yield(p) {
				return
			}
		}
	}
	get := func(k string) (int, bool) { v, ok := right[k]; return v, ok }
	if !EqualMaps(len(left), len(right), iter, get, func(a, b int) bool { return a == b }) {
		t.Fatal("equal maps reported unequal")
	}
	right["b"] = 3
	if EqualMaps(len(left), len(right), iter, get, func(a, b int) bool { return a == b }) {
		t.Fatal("unequal maps reported equal")
	}
}

func TestEqualMapsStopsOnMismatch(t *testing.T) {
	visited := 0
	iter := func(yield func(Pair[string, int]) bool) {
		for _, p := range []Pair[string, int]{{Key: "a", Value: 1}, {Key: "b", Value: 2}} {
			visited++
			if !yield(p) {
				return
			}
		}
	}
	get := func(k string) (int, bool) { return 0, false }
	if EqualMaps(2, 2, iter, get, func(a, b int) bool { return a == b }) {
		t.Fatal("missing key maps reported equal")
	}
	if visited != 1 {
		t.Fatalf("visited = %d, want early stop after first mismatch", visited)
	}
}
