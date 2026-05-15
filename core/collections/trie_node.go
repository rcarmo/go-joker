package collections

// TrieBranching is the branching factor used by persistent vector trie nodes.
const TrieBranching = 32

// TrieNode is a generic fixed-width trie node. Values are intentionally opaque
// so callers can store either child nodes or leaf values without importing root
// collection/object packages.
type TrieNode struct {
	arr [TrieBranching]any
}

// NewTrieNode returns an empty trie node.
func NewTrieNode() *TrieNode { return &TrieNode{} }

// Get returns the slot value at idx.
func (n *TrieNode) Get(idx int) any { return n.arr[idx] }

// Set assigns the slot value at idx.
func (n *TrieNode) Set(idx int, val any) { n.arr[idx] = val }

// CloneTrieNode returns a shallow copy of n.
func CloneTrieNode(n *TrieNode) *TrieNode {
	ret := &TrieNode{}
	if n != nil {
		*ret = *n
	}
	return ret
}

// NewTriePath creates a leftmost path of internal nodes ending in leaf.
func NewTriePath(level uint, shift uint, leaf *TrieNode) *TrieNode {
	if level == 0 {
		return leaf
	}
	ret := NewTrieNode()
	ret.Set(0, NewTriePath(level-shift, shift, leaf))
	return ret
}
