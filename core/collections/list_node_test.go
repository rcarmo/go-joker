package collections

import "testing"

func TestListNodePersistentCons(t *testing.T) {
	empty := NewEmptyListNode(0)
	if !empty.IsEmpty() || empty.Count() != 0 || empty.Rest() != empty {
		t.Fatalf("empty node = count %d rest self %v", empty.Count(), empty.Rest() == empty)
	}
	one := NewListNode(1, empty)
	two := NewListNode(2, one)
	if two.IsEmpty() || two.Count() != 2 || two.First() != 2 || two.Rest().First() != 1 || two.Rest().Rest() != empty {
		t.Fatalf("unexpected list chain: %#v", two)
	}
	if one.Count() != 1 || empty.Count() != 0 {
		t.Fatalf("persistent counts mutated: one=%d empty=%d", one.Count(), empty.Count())
	}
}
