package core

import corecollections "github.com/rcarmo/go-joker/core/collections"

// Persistent Vector — Clojure-style 32-way branching trie with tail optimization.
// Provides O(log32 n) assoc and O(1) amortized conj via structural sharing.
//
// Design (Bagwell 2001 / Hickey):
// - 32-way branching (shift by 5 bits per level)
// - Tail: last ≤32 elements stored flat (fast append)
// - Path copying: assoc only copies nodes on the root-to-leaf path
// - Structural sharing: unchanged subtrees are shared between versions

const pvBranching = corecollections.TrieBranching
const pvShift = 5
const pvMask = 0x1f

// PersistentVector is an immutable vector backed by a 32-way trie.
type PersistentVector struct {
	InfoHolder
	MetaHolder
	count int
	shift uint // bits to shift for root level (5 * depth)
	root  *corecollections.TrieNode
	tail  []Object
}

var emptyPVNode = corecollections.NewTrieNode()

// EmptyPersistentVector returns a new empty persistent vector.
func EmptyPersistentVector() *PersistentVector {
	return &PersistentVector{
		count: 0,
		shift: pvShift,
		root:  emptyPVNode,
		tail:  make([]Object, 0, pvBranching),
	}
}

// PersistentVectorFrom creates a PersistentVector from a slice.
func PersistentVectorFrom(items []Object) *PersistentVector {
	pv := EmptyPersistentVector()
	for _, item := range items {
		pv = pv.Conj(item)
	}
	return pv
}

// Count returns the number of elements.
func (v *PersistentVector) Count() int {
	return v.count
}

// tailOffset returns the index where the tail starts.
func (v *PersistentVector) tailOffset() int {
	if v.count < pvBranching {
		return 0
	}
	return ((v.count - 1) >> pvShift) << pvShift
}

// arrayFor returns the leaf array containing index i.
func (v *PersistentVector) arrayFor(i int) []Object {
	if i >= v.tailOffset() {
		return v.tail
	}
	node := v.root
	for level := v.shift; level > 0; level -= pvShift {
		node = node.Get((i >> level) & pvMask).(*corecollections.TrieNode)
	}
	// Convert leaf node to Object slice
	leafStart := (i >> pvShift) << pvShift
	leafEnd := leafStart + pvBranching
	if leafEnd > v.tailOffset() {
		leafEnd = v.tailOffset()
	}
	result := make([]Object, leafEnd-leafStart)
	for j := range result {
		result[j] = node.Get(j).(Object)
	}
	return result
}

// Nth returns the element at index i.
func (v *PersistentVector) Nth(i int) Object {
	if i < 0 || i >= v.count {
		panic(RT.NewError("Index out of bounds"))
	}
	if i >= v.tailOffset() {
		return v.tail[i-v.tailOffset()]
	}
	node := v.root
	for level := v.shift; level > 0; level -= pvShift {
		node = node.Get((i >> level) & pvMask).(*corecollections.TrieNode)
	}
	return node.Get(i & pvMask).(Object)
}

// Conj appends an element, returning a new vector.
func (v *PersistentVector) Conj(val Object) *PersistentVector {
	// Room in tail?
	if v.count-v.tailOffset() < pvBranching {
		newTail := corecollections.AppendCopy(v.tail, val)
		return &PersistentVector{
			count: v.count + 1,
			shift: v.shift,
			root:  v.root,
			tail:  newTail,
		}
	}
	// Tail is full — push tail into trie, start new tail
	var newRoot *corecollections.TrieNode
	newShift := v.shift
	tailNode := corecollections.NewTrieNode()
	for i, obj := range v.tail {
		tailNode.Set(i, obj)
	}
	// Is there room in the existing trie?
	if (v.count >> pvShift) > (1 << v.shift) {
		// Trie overflow — add a new level
		newRoot = corecollections.NewTrieNode()
		newRoot.Set(0, v.root)
		newRoot.Set(1, pvNewPath(v.shift, tailNode))
		newShift += pvShift
	} else {
		newRoot = v.pushTail(v.shift, v.root, tailNode)
	}
	return &PersistentVector{
		count: v.count + 1,
		shift: newShift,
		root:  newRoot,
		tail:  []Object{val},
	}
}

// pushTail inserts a tail node into the trie.
func (v *PersistentVector) pushTail(level uint, parent *corecollections.TrieNode, tailNode *corecollections.TrieNode) *corecollections.TrieNode {
	subIdx := ((v.count - 1) >> level) & pvMask
	ret := cloneNode(parent)
	if level == pvShift {
		ret.Set(subIdx, tailNode)
	} else {
		child := parent.Get(subIdx)
		if child != nil {
			ret.Set(subIdx, v.pushTail(level-pvShift, child.(*corecollections.TrieNode), tailNode))
		} else {
			ret.Set(subIdx, pvNewPath(level-pvShift, tailNode))
		}
	}
	return ret
}

// Assoc returns a new vector with index i set to val.
func (v *PersistentVector) Assoc(i int, val Object) *PersistentVector {
	if i < 0 || i > v.count {
		panic(RT.NewError("Index out of bounds"))
	}
	if i == v.count {
		return v.Conj(val)
	}
	if i >= v.tailOffset() {
		newTail := corecollections.AssocCopy(v.tail, i-v.tailOffset(), val)
		return &PersistentVector{
			count: v.count,
			shift: v.shift,
			root:  v.root,
			tail:  newTail,
		}
	}
	newRoot := v.assocNode(v.shift, v.root, i, val)
	return &PersistentVector{
		count: v.count,
		shift: v.shift,
		root:  newRoot,
		tail:  v.tail,
	}
}

func (v *PersistentVector) assocNode(level uint, node *corecollections.TrieNode, i int, val Object) *corecollections.TrieNode {
	ret := cloneNode(node)
	if level == 0 {
		ret.Set(i&pvMask, val)
	} else {
		subIdx := (i >> level) & pvMask
		ret.Set(subIdx, v.assocNode(level-pvShift, node.Get(subIdx).(*corecollections.TrieNode), i, val))
	}
	return ret
}

// Pop returns a new vector without the last element.
func (v *PersistentVector) Pop() *PersistentVector {
	if v.count == 0 {
		panic(RT.NewError("Can't pop empty vector"))
	}
	if v.count == 1 {
		return EmptyPersistentVector()
	}
	// More than one element in tail?
	if v.count-v.tailOffset() > 1 {
		newTail := corecollections.PopCopy(v.tail)
		return &PersistentVector{
			count: v.count - 1,
			shift: v.shift,
			root:  v.root,
			tail:  newTail,
		}
	}
	// Tail has exactly 1 element — pop tail from trie
	newTail := v.arrayFor(v.count - 2)
	newRoot := v.popTail(v.shift, v.root)
	newShift := v.shift
	if newRoot == nil {
		newRoot = emptyPVNode
	}
	if v.shift > pvShift && newRoot.Get(1) == nil {
		newRoot = newRoot.Get(0).(*corecollections.TrieNode)
		newShift -= pvShift
	}
	return &PersistentVector{
		count: v.count - 1,
		shift: newShift,
		root:  newRoot,
		tail:  newTail,
	}
}

func (v *PersistentVector) popTail(level uint, node *corecollections.TrieNode) *corecollections.TrieNode {
	subIdx := ((v.count - 2) >> level) & pvMask
	if level > pvShift {
		newChild := v.popTail(level-pvShift, node.Get(subIdx).(*corecollections.TrieNode))
		if newChild == nil && subIdx == 0 {
			return nil
		}
		ret := cloneNode(node)
		ret.Set(subIdx, newChild)
		return ret
	} else if subIdx == 0 {
		return nil
	} else {
		ret := cloneNode(node)
		ret.Set(subIdx, nil)
		return ret
	}
}

// ToSlice returns all elements as a flat slice.
func (v *PersistentVector) ToSlice() []Object {
	result := make([]Object, v.count)
	for i := 0; i < v.count; i++ {
		result[i] = v.Nth(i)
	}
	return result
}

// --- Helpers ---

func cloneNode(node *corecollections.TrieNode) *corecollections.TrieNode {
	return corecollections.CloneTrieNode(node)
}

func pvNewPath(level uint, node *corecollections.TrieNode) *corecollections.TrieNode {
	return corecollections.NewTriePath(level, pvShift, node)
}

// --- Object interface ---

func (v *PersistentVector) At(i int) Object { return v.Nth(i) }

func (v *PersistentVector) Seq() Seq { return NewVectorFrom(v.ToSlice()...).Seq() }

func (v *PersistentVector) ToString(escape bool) string { return CountedIndexedToString(v, escape) }

func (v *PersistentVector) Equals(other interface{}) bool {
	if v == other {
		return true
	}
	switch other := other.(type) {
	case CountedIndexed:
		return AreCountedIndexedEqual(v, other)
	default:
		return IsSeqEqual(v.Seq(), other)
	}
}

func (v *PersistentVector) WithInfo(info *ObjectInfo) Object {
	res := *v
	res.info = info
	return &res
}

func (v *PersistentVector) WithMeta(meta Map) Object {
	res := *v
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func (v *PersistentVector) GetType() *Type { return TYPE.ArrayVector }
func (v *PersistentVector) Hash() uint32   { return CountedIndexedHash(v) }
