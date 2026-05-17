package collections

import "testing"

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
