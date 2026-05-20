package collections

import "testing"

func TestBitmapHelpers(t *testing.T) {
	if got := BitCount(0); got != 0 {
		t.Fatalf("BitCount(0) = %d, want 0", got)
	}
	if got := BitCount(0b101101); got != 4 {
		t.Fatalf("BitCount(0b101101) = %d, want 4", got)
	}
	var hash uint32 = 0b101011_00010
	if got := HashMask(hash, 0); got != 2 {
		t.Fatalf("HashMask low = %d, want 2", got)
	}
	if got := HashMask(hash, 5); got != 0b101011&0x1f {
		t.Fatalf("HashMask shifted = %d, want %d", got, 0b101011&0x1f)
	}
	if got := Bitpos(2, 0); got != 4 {
		t.Fatalf("Bitpos(2,0) = %d, want 4", got)
	}
}

func TestHashMapPairStorageOperations(t *testing.T) {
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

func TestHashMapAssoc2Copy(t *testing.T) {
	src := []string{"a", "b", "c"}
	assoc2 := Assoc2Copy(src, 0, "A", 2, "C")
	if assoc2[0] != "A" || assoc2[2] != "C" || src[0] != "a" || src[2] != "c" {
		t.Fatalf("Assoc2Copy result/source = %#v / %#v", assoc2, src)
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
