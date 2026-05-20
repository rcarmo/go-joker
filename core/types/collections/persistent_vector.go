package collections

import (
	"io"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

// Persistent coretypes.Vector — Clojure-style 32-way branching trie with tail optimization.
// Provides O(log32 n) assoc and O(1) amortized conj via structural sharing.
//
// Design (Bagwell 2001 / Hickey):
// - 32-way branching (shift by 5 bits per level)
// - Tail: last ≤32 elements stored flat (fast append)
// - Path copying: assoc only copies nodes on the root-to-leaf path
// - Structural sharing: unchanged subtrees are shared between versions

const pvBranching = TrieBranching
const pvShift = 5
const pvMask = 0x1f

// PersistentVector is an immutable vector backed by a 32-way trie.
type PersistentVector struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	count int
	shift uint // bits to shift for root level (5 * depth)
	root  *TrieNode
	tail  []coretypes.Object
}

type PersistentVectorSeq struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	vector *PersistentVector
	index  int
}

type PersistentVectorConsSeq struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	first coretypes.Object
	rest  coretypes.Seq
}

var emptyPVNode = NewTrieNode()

// EmptyPersistentVector returns a new empty persistent vector.
func EmptyPersistentVector() *PersistentVector {
	return &PersistentVector{
		count: 0,
		shift: pvShift,
		root:  emptyPVNode,
		tail:  make([]coretypes.Object, 0, pvBranching),
	}
}

// PersistentVectorFrom creates a PersistentVector from a slice.
func PersistentVectorFrom(items []coretypes.Object) *PersistentVector {
	pv := EmptyPersistentVector()
	for _, item := range items {
		pv = pv.Conjoin(item)
	}
	return pv
}

// Count returns the number of elements.
func (v *PersistentVector) Count() int {
	return v.count
}

// tailOffset returns the index where the tail starts.
func (v *PersistentVector) tailOffset() int {
	return PersistentVectorTailOffset(v.count)
}

// arrayFor returns the leaf array containing index i.
func (v *PersistentVector) arrayFor(i int) []coretypes.Object {
	return PersistentVectorArrayFor(v.count, v.shift, v.root, v.tail, i)
}

// Nth returns the element at index i.
func (v *PersistentVector) Nth(i int) coretypes.Object {
	if value, ok := PersistentVectorNth(v.count, v.tailOffset(), v.tail, v.shift, v.root, i); ok {
		return value
	}
	panic(coretypes.RuntimeError("Index out of bounds"))
}

// Conjoin appends an element, returning a new vector.
func (v *PersistentVector) Conjoin(val coretypes.Object) *PersistentVector {
	// Room in tail?
	if v.count-v.tailOffset() < pvBranching {
		newTail := AppendCopy(v.tail, val)
		return &PersistentVector{
			count: v.count + 1,
			shift: v.shift,
			root:  v.root,
			tail:  newTail,
		}
	}
	// Tail is full — push tail into trie, start new tail
	var newRoot *TrieNode
	newShift := v.shift
	tailNode := PersistentVectorTailNode(v.tail)
	// Is there room in the existing trie?
	if PersistentVectorTrieOverflow(v.count, v.shift) {
		// Trie overflow — add a new level
		newRoot = NewTrieNode()
		newRoot.Set(0, v.root)
		newRoot.Set(1, NewTriePath(v.shift, pvShift, tailNode))
		newShift += pvShift
	} else {
		newRoot = v.pushTail(v.shift, v.root, tailNode)
	}
	return &PersistentVector{
		count: v.count + 1,
		shift: newShift,
		root:  newRoot,
		tail:  []coretypes.Object{val},
	}
}

// pushTail inserts a tail node into the trie.
func (v *PersistentVector) pushTail(level uint, parent *TrieNode, tailNode *TrieNode) *TrieNode {
	return PersistentVectorPushTail(level, v.count, parent, tailNode)
}

// AssocIndex returns a new vector with index i set to val.
func (v *PersistentVector) AssocIndex(i int, val coretypes.Object) *PersistentVector {
	if i < 0 || i > v.count {
		panic(coretypes.RuntimeError("Index out of bounds"))
	}
	if i == v.count {
		return v.Conjoin(val)
	}
	if i >= v.tailOffset() {
		newTail := AssocCopy(v.tail, i-v.tailOffset(), val)
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

func (v *PersistentVector) assocNode(level uint, node *TrieNode, i int, val coretypes.Object) *TrieNode {
	return PersistentVectorAssocNode(level, node, i, val)
}

// Pop returns a new vector without the last element.
func (v *PersistentVector) Pop() coretypes.Stack {
	if v.count == 0 {
		panic(coretypes.RuntimeError("Can't pop empty vector"))
	}
	if v.count == 1 {
		return EmptyPersistentVector()
	}
	// More than one element in tail?
	if PersistentVectorPopTailOnly(v.count, v.tailOffset()) {
		newTail := PopCopy(v.tail)
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
	if adjustedRoot, adjustedShift, useEmptyRoot := PersistentVectorPopShift(v.shift, newRoot, pvShift); useEmptyRoot {
		newRoot = emptyPVNode
	} else {
		newRoot = adjustedRoot
		newShift = adjustedShift
	}
	return &PersistentVector{
		count: v.count - 1,
		shift: newShift,
		root:  newRoot,
		tail:  newTail,
	}
}

func (v *PersistentVector) popTail(level uint, node *TrieNode) *TrieNode {
	return PersistentVectorPopTail(level, v.count, node)
}

// ToSlice returns all elements as a flat slice.
func (v *PersistentVector) ToSlice() []coretypes.Object {
	return PersistentVectorToSlice(v.count, func(i int) coretypes.Object { return v.Nth(i) })
}

// --- coretypes.Object interface ---

func (v *PersistentVector) At(i int) coretypes.Object { return v.Nth(i) }

func (v *PersistentVector) SequentialMarker() {}

func (v *PersistentVector) TryNth(i int, d coretypes.Object) coretypes.Object {
	if i < 0 || i >= v.count {
		return d
	}
	return v.Nth(i)
}

func (v *PersistentVector) Get(key coretypes.Object) (bool, coretypes.Object) {
	return IndexedGetByObject[coretypes.Object](v, key)
}

func (v *PersistentVector) EntryAt(key coretypes.Object) coretypes.Object {
	if ok, val := v.Get(key); ok {
		return NewArrayVectorFrom(key, val)
	}
	return nil
}

func (v *PersistentVector) Compare(other coretypes.Object) int {
	v2 := coretypes.EnsureObjectIsCountedIndexed(coretypes.RootObject(other), "Cannot compare coretypes.Vector: %s")
	return IndexedCompare[coretypes.Object](v, v2, func(a, b coretypes.Object) int { return coretypes.EnsureObjectIsComparable(a, "").Compare(b) })
}

func (v *PersistentVector) Peek() coretypes.Object {
	if v.count > 0 {
		return v.Nth(v.count - 1)
	}
	return coretypes.RuntimeNil
}

func (v *PersistentVector) Rseq() coretypes.Seq { return NewVectorFrom(v.ToSlice()...).Rseq() }

func (v *PersistentVector) Call(args []coretypes.Object) coretypes.Object {
	if len(args) != 1 {
		coretypes.RuntimePanicArityMinMax(len(args), 1, 1)
	}
	i, ok := IndexFromObject(args[0])
	if !ok {
		panic(coretypes.RuntimeError("Key must be integer"))
	}
	return v.Nth(i)
}

func (v *PersistentVector) Empty() coretypes.Collection { return EmptyPersistentVector() }

func (v *PersistentVector) Pprint(w io.Writer, indent int) int {
	return IndexedPprint[coretypes.Object](v, w, indent, coretypes.RuntimePprintObject, coretypes.RuntimeWriteIndent)
}

func (v *PersistentVector) Format(w io.Writer, indent int) int {
	return IndexedFormat[coretypes.Object](v, w, indent, coretypes.RuntimeFormatObject, coretypes.RuntimeMaybeNewLine, coretypes.RuntimeIsComment, coretypes.RuntimeWriteIndent)
}

func (v *PersistentVector) Conj(val coretypes.Object) coretypes.Conjable { return v.Conjoin(val) }

func (v *PersistentVector) Assoc(key, val coretypes.Object) coretypes.Associative {
	i, ok := IndexFromObject(key)
	if !ok {
		panic(coretypes.RuntimeError("Key must be integer"))
	}
	return v.AssocIndex(i, val)
}

func (v *PersistentVector) Seq() coretypes.Seq { return &PersistentVectorSeq{vector: v, index: 0} }

func (seq *PersistentVectorSeq) Seq() coretypes.Seq { return seq }
func (seq *PersistentVectorSeq) ToString(escape bool) string {
	return SeqToString(seq, func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (seq *PersistentVectorSeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(seq, other)
}
func (seq *PersistentVectorSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	seq.Info = info
	return seq
}
func (seq *PersistentVectorSeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *seq
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}
func (seq *PersistentVectorSeq) GetType() *coretypes.Type { return coretypes.RuntimeTypes.VectorSeq }
func (seq *PersistentVectorSeq) Hash() uint32             { return HashOrdered(seq) }
func (seq *PersistentVectorSeq) First() coretypes.Object {
	if seq.IsEmpty() {
		return coretypes.RuntimeNil
	}
	return seq.vector.Nth(seq.index)
}
func (seq *PersistentVectorSeq) Rest() coretypes.Seq {
	if seq.vector == nil {
		return seq
	}
	if seq.index+1 < seq.vector.Count() {
		return &PersistentVectorSeq{vector: seq.vector, index: seq.index + 1}
	}
	return &PersistentVectorSeq{vector: seq.vector, index: seq.vector.Count()}
}
func (seq *PersistentVectorSeq) IsEmpty() bool {
	return seq.vector == nil || seq.index >= seq.vector.Count()
}
func (seq *PersistentVectorSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &PersistentVectorConsSeq{first: obj, rest: seq}
}

func (seq *PersistentVectorConsSeq) Seq() coretypes.Seq { return seq }
func (seq *PersistentVectorConsSeq) ToString(escape bool) string {
	return SeqToString(seq, func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (seq *PersistentVectorConsSeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(seq, other)
}
func (seq *PersistentVectorConsSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	seq.Info = info
	return seq
}
func (seq *PersistentVectorConsSeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *seq
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}
func (seq *PersistentVectorConsSeq) GetType() *coretypes.Type { return coretypes.RuntimeTypes.ConsSeq }
func (seq *PersistentVectorConsSeq) Hash() uint32             { return HashOrdered(seq) }
func (seq *PersistentVectorConsSeq) First() coretypes.Object  { return seq.first }
func (seq *PersistentVectorConsSeq) Rest() coretypes.Seq      { return seq.rest }
func (seq *PersistentVectorConsSeq) IsEmpty() bool            { return false }
func (seq *PersistentVectorConsSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &PersistentVectorConsSeq{first: obj, rest: seq}
}

func (v *PersistentVector) ToString(escape bool) string {
	return IndexedToString[coretypes.Object](v, escape)
}

func (v *PersistentVector) Equals(other interface{}) bool {
	if v == other {
		return true
	}
	switch other := other.(type) {
	case coretypes.CountedIndexed:
		return IndexedEqual[coretypes.Object](v, other)
	default:
		return coretypes.IsSeqEqual(v.Seq(), other)
	}
}

func (v *PersistentVector) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *v
	res.Info = info
	return &res
}

func (v *PersistentVector) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *v
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (v *PersistentVector) GetType() *coretypes.Type { return coretypes.RuntimeTypes.ArrayVector }
func (v *PersistentVector) Hash() uint32             { return IndexedHash[coretypes.Object](v) }
