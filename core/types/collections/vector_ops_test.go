package collections

import "testing"

func TestVectorStorageCopyOperations(t *testing.T) {
	src := []string{"a", "b", "c"}
	appended := AppendCopy(src, "d")
	if got := len(appended); got != 4 {
		t.Fatalf("AppendCopy len = %d, want 4", got)
	}
	if len(src) != 3 || src[2] != "c" {
		t.Fatalf("AppendCopy mutated source: %#v", src)
	}

	popped := PopCopy(src)
	if len(popped) != 2 || popped[1] != "b" || len(src) != 3 {
		t.Fatalf("PopCopy result/source = %#v / %#v", popped, src)
	}
}

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

func TestCloneSlicePreservesLengthCapacityAndDetaches(t *testing.T) {
	src := make([]int, 2, 4)
	src[0], src[1] = 1, 2
	dst := CloneSlice(src)
	if len(dst) != 2 || cap(dst) != 4 {
		t.Fatalf("CloneSlice len/cap = %d/%d, want 2/4", len(dst), cap(dst))
	}
	dst[0] = 99
	if src[0] != 1 {
		t.Fatal("CloneSlice should detach backing storage")
	}
}

func TestAssocCopyDetachesSource(t *testing.T) {
	src := []string{"a", "b", "c"}
	assoc := AssocCopy(src, 1, "B")
	if assoc[1] != "B" || src[1] != "b" {
		t.Fatalf("AssocCopy result/source = %#v / %#v", assoc, src)
	}
}

func TestFromValuesReturnsFreshSlice(t *testing.T) {
	src := []int{1, 2, 3}
	dst := FromValues(src...)
	dst[0] = 99
	if src[0] != 1 {
		t.Fatal("FromValues should detach source values")
	}
	if empty := FromValues[int](); empty != nil {
		t.Fatalf("empty FromValues = %#v, want nil", empty)
	}
}
