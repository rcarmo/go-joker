package collections

import "testing"

func TestPairStorageOperations(t *testing.T) {
	pairs := []string{"a", "1", "b", "2", "c", "3"}

	removed := RemovePair(pairs, 1)
	if got, want := removed, []string{"a", "1", "c", "3"}; len(got) != len(want) || got[0] != want[0] || got[2] != want[2] {
		t.Fatalf("RemovePair = %#v, want %#v", got, want)
	}
	if pairs[2] != "b" || len(pairs) != 6 {
		t.Fatalf("RemovePair mutated source: %#v", pairs)
	}

	appended := AppendPair(pairs, "d", "4")
	if len(appended) != 8 || appended[6] != "d" || appended[7] != "4" || len(pairs) != 6 {
		t.Fatalf("AppendPair result/source = %#v / %#v", appended, pairs)
	}

	inserted := InsertPair(pairs, 1, "x", "9")
	want := []string{"a", "1", "x", "9", "b", "2", "c", "3"}
	if len(inserted) != len(want) {
		t.Fatalf("InsertPair len = %d, want %d", len(inserted), len(want))
	}
	for i := range want {
		if inserted[i] != want[i] {
			t.Fatalf("InsertPair[%d] = %q, want %q (all %#v)", i, inserted[i], want[i], inserted)
		}
	}
}

func TestPackIndexedNodes(t *testing.T) {
	nodes := []string{"", "a", "", "skip", "b"}
	bitmap, packed := PackIndexedNodes(nodes, 3, func(s string) bool { return s != "" })
	if bitmap != (1<<1)|(1<<4) {
		t.Fatalf("bitmap = %b", bitmap)
	}
	want := []interface{}{nil, "a", nil, "b"}
	if len(packed) != len(want) {
		t.Fatalf("packed len = %d, want %d: %#v", len(packed), len(want), packed)
	}
	for i := range want {
		if packed[i] != want[i] {
			t.Fatalf("packed[%d] = %#v, want %#v (all %#v)", i, packed[i], want[i], packed)
		}
	}
}
