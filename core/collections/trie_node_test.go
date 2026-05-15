package collections

import "testing"

func TestCloneTrieNode(t *testing.T) {
	n := NewTrieNode()
	n.Set(3, "leaf")
	clone := CloneTrieNode(n)
	if clone == n {
		t.Fatal("CloneTrieNode should return a distinct node")
	}
	if got := clone.Get(3); got != "leaf" {
		t.Fatalf("clone slot = %v, want leaf", got)
	}
	clone.Set(3, "changed")
	if got := n.Get(3); got != "leaf" {
		t.Fatalf("clone mutation changed source slot to %v", got)
	}
}

func TestNewTriePath(t *testing.T) {
	leaf := NewTrieNode()
	leaf.Set(0, "value")
	path := NewTriePath(10, 5, leaf)
	first, ok := path.Get(0).(*TrieNode)
	if !ok {
		t.Fatalf("first path slot = %T, want *TrieNode", path.Get(0))
	}
	gotLeaf, ok := first.Get(0).(*TrieNode)
	if !ok || gotLeaf != leaf {
		t.Fatalf("leaf = %v/%T, want original leaf", gotLeaf, gotLeaf)
	}
}
