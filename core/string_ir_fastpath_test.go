package core

import "testing"

func TestStringNthFastUnicodeCorrectness(t *testing.T) {
	got := stringNthFast("abcdef", 3)
	if ch, ok := got.(Char); !ok || ch.Ch != 'd' {
		t.Fatalf("expected d, got %T %s", got, got.ToString(false))
	}
	got = stringNthFast("éclair", 1)
	if ch, ok := got.(Char); !ok || ch.Ch != 'c' {
		t.Fatalf("expected c, got %T %s", got, got.ToString(false))
	}
}

func TestIRNthStringFastPath(t *testing.T) {
	requireString(t, evalTestScript(t, `(loop [i 0 s ""]
  (if (= i 3)
    s
    (recur (inc i) (str s (nth "abcdef" i)))))`), "abc")
}
