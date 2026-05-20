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

func TestFormatDelimited(t *testing.T) {
	items := []string{"a", "b", "c"}
	got := FormatDelimited("[", "]", ", ", func(yield func(string) bool) {
		for _, item := range items {
			if !yield(item) {
				return
			}
		}
	})
	if got != "[a, b, c]" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatPairDelimited(t *testing.T) {
	pairs := []Pair[string, string]{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}}
	got := FormatPairDelimited("{", "}", " ", ", ", func(yield func(Pair[string, string]) bool) {
		for _, p := range pairs {
			if !yield(p) {
				return
			}
		}
	}, func(k string) string { return k }, func(v string) string { return v })
	if got != "{a 1, b 2}" {
		t.Fatalf("got %q", got)
	}
}
