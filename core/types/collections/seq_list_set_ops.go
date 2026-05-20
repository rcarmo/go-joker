package collections

import (
	"fmt"
	"io"

	githubhash "github.com/rcarmo/go-joker/core/hashutil"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

// ListNode is root-independent persistent list storage. Root core owns Object,
// metadata, sequence protocols, printing, hashing, and error behavior.
type ListNode[T any] struct {
	first T
	rest  *ListNode[T]
	count int
}

func NewEmptyListNode[T any](zero T) *ListNode[T] {
	n := &ListNode[T]{first: zero}
	n.rest = n
	return n
}

func NewListNode[T any](first T, rest *ListNode[T]) *ListNode[T] {
	n := &ListNode[T]{first: first, rest: rest}
	if rest != nil {
		n.count = rest.count + 1
	}
	return n
}

func (n *ListNode[T]) First() T           { return n.first }
func (n *ListNode[T]) Rest() *ListNode[T] { return n.rest }
func (n *ListNode[T]) Count() int         { return n.count }
func (n *ListNode[T]) IsEmpty() bool      { return n == nil || n.count == 0 }

// MaterializeListNode builds (or reuses) the list-node representation for a
// root list adapter without importing root object/runtime types.
func MaterializeListNode[T any](current *ListNode[T], count int, first T, rest *ListNode[T]) *ListNode[T] {
	if current != nil {
		return current
	}
	if count == 0 {
		return NewEmptyListNode(first)
	}
	return NewListNode(first, rest)
}

func ListSecond[T any](node *ListNode[T]) T {
	return node.Rest().First()
}

func ListThird[T any](node *ListNode[T]) T {
	return node.Rest().Rest().First()
}

func ListFourth[T any](node *ListNode[T]) T {
	return node.Rest().Rest().Rest().First()
}

type SeqIterator struct{ seq coretypes.Seq }

func NewSeqIterator(seq coretypes.Seq) *SeqIterator { return &SeqIterator{seq: seq} }
func (iter *SeqIterator) Next() coretypes.Object {
	res := iter.seq.First()
	iter.seq = iter.seq.Rest()
	return res
}
func (iter *SeqIterator) HasNext() bool { return !iter.seq.IsEmpty() }

func SeqSecond(seq coretypes.Seq) coretypes.Object { return seq.Rest().First() }
func SeqThird(seq coretypes.Seq) coretypes.Object  { return seq.Rest().Rest().First() }
func SeqFourth(seq coretypes.Seq) coretypes.Object { return seq.Rest().Rest().Rest().First() }

func SeqToSlice(seq coretypes.Seq) []coretypes.Object {
	res := make([]coretypes.Object, 0)
	for !seq.IsEmpty() {
		res = append(res, seq.First())
		seq = seq.Rest()
	}
	return res
}

func SeqToSliceN(seq coretypes.Seq, n int) []coretypes.Object {
	objs := make([]coretypes.Object, n)
	for i := 0; i < n; i++ {
		objs[i] = seq.First()
		seq = seq.Rest()
	}
	return objs
}

func SeqCount(seq coretypes.Seq) int {
	if c, ok := seq.(coretypes.Counted); ok {
		return c.Count()
	}
	n := 0
	for !seq.IsEmpty() {
		if c, ok := seq.(coretypes.Counted); ok {
			return n + c.Count()
		}
		n++
		seq = seq.Rest()
	}
	return n
}
func HashUnordered(seq coretypes.Seq, seed uint32) uint32 {
	for !seq.IsEmpty() {
		seed += seq.First().Hash()
		seq = seq.Rest()
	}
	h := coretypes.NewHash32()
	h.Write(githubhash.Uint32Bytes(seed))
	return h.Sum32()
}
func HashOrdered(seq coretypes.Seq) uint32 {
	h := coretypes.NewHash32()
	for !seq.IsEmpty() {
		h.Write(githubhash.Uint32Bytes(seq.First().Hash()))
		seq = seq.Rest()
	}
	return h.Sum32()
}

func SeqToString(seq coretypes.Seq, toString func(coretypes.Object) string) string {
	return FormatDelimited("(", ")", " ", func(yield func(string) bool) {
		for it := NewSeqIterator(seq); it.HasNext(); {
			if !yield(toString(it.Next())) {
				return
			}
		}
	})
}
func SeqNthTry(seq coretypes.Seq, n int) (coretypes.Object, bool) {
	if n < 0 {
		return nil, false
	}
	i := n
	for !seq.IsEmpty() {
		if i == 0 {
			return seq.First(), true
		}
		seq = seq.Rest()
		i--
	}
	return nil, false
}
func SeqNthError(n int, length int) string {
	return fmt.Sprintf("Index %d exceeds seq's length %d", n, length)
}
func SeqNthArray[T any](arr []T, index int, n int) (T, int, bool) {
	idx := index + n
	if idx >= 0 && idx < len(arr) {
		return arr[idx], len(arr) - index, true
	}
	var zero T
	return zero, len(arr) - index, false
}

func ArraySeqFirst[T any](arr []T, index int) (T, bool) {
	if index < 0 || index >= len(arr) {
		var zero T
		return zero, false
	}
	return arr[index], true
}
func ArraySeqRest(index int, length int) (next int, ok bool) {
	next = index + 1
	return next, next < length
}
func ArraySeqIsEmpty(index int, length int) bool { return index >= length }
func ArraySeqCount(index int, length int) int {
	n := length - index
	if n < 0 {
		return 0
	}
	return n
}

func BuildListFromReverse[T any, L any](items []T, empty L, conj func(L, T) L) L {
	res := empty
	for i := len(items) - 1; i >= 0; i-- {
		res = conj(res, items[i])
	}
	return res
}
func ListPop[T any](count int, rest T, errf func(string) any) T {
	if count == 0 {
		panic(errf("Can't pop empty list"))
	}
	return rest
}

func EnsureSetMap(m coretypes.Map, empty func() coretypes.Map) coretypes.Map {
	if m == nil {
		return empty()
	}
	return m
}
func SetGet(m coretypes.Map, key coretypes.Object) (bool, coretypes.Object) {
	if m == nil {
		return false, nil
	}
	if ok, _ := m.Get(key); ok {
		return true, key
	}
	return false, nil
}
func SetCount(m coretypes.Map) int {
	if m == nil {
		return 0
	}
	return m.Count()
}
func SetSeq(m coretypes.Map, empty coretypes.Seq) coretypes.Seq {
	if m == nil {
		return empty
	}
	return m.Keys()
}
func SetAddViaMap(m coretypes.Map, obj coretypes.Object, exists func(coretypes.Map, coretypes.Object) bool) (coretypes.Map, bool) {
	if exists != nil && exists(m, obj) {
		return m, false
	}
	return m.Assoc(obj, coretypes.Boolean{B: true}).(coretypes.Map), true
}
func SetFromSeq[S any](seq coretypes.Seq, empty S, add func(S, coretypes.Object) S) S {
	res := empty
	for !seq.IsEmpty() {
		res = add(res, seq.First())
		seq = seq.Rest()
	}
	return res
}

func SetPprint(seq coretypes.Seq, w io.Writer, indent int, pprint func(coretypes.Object, int, io.Writer) int, writeIndent func(io.Writer, int)) int {
	i := indent + 1
	fmt.Fprint(w, "#{")
	for it := NewSeqIterator(seq); it.HasNext(); {
		i = pprint(it.Next(), indent+2, w)
		if it.HasNext() {
			fmt.Fprint(w, "\n")
			writeIndent(w, indent+2)
		}
	}
	fmt.Fprint(w, "}")
	return i + 1
}
func SetFormat(seq coretypes.Seq, w io.Writer, indent int, format func(coretypes.Object, int, io.Writer) int, maybeNewLine func(io.Writer, coretypes.Object, coretypes.Object, int, int) int, isComment func(coretypes.Object) bool, writeIndent func(io.Writer, int)) int {
	i := indent + 2
	fmt.Fprint(w, "#{")
	var prev coretypes.Object
	for it := NewSeqIterator(seq); it.HasNext(); {
		obj := it.Next()
		if prev != nil {
			i = maybeNewLine(w, prev, obj, indent+2, i)
		}
		i = format(obj, i, w)
		prev = obj
	}
	if prev != nil && isComment(prev) {
		fmt.Fprint(w, "\n")
		writeIndent(w, indent+2)
		i = indent + 2
	}
	fmt.Fprint(w, "}")
	return i + 1
}

func TransientMapConjEntry(obj coretypes.Object) (coretypes.Object, coretypes.Object, bool) {
	seq, ok := obj.(coretypes.Seqable)
	if !ok {
		return nil, nil, false
	}
	s := seq.Seq()
	if s.IsEmpty() {
		return nil, nil, false
	}
	k := s.First()
	s = s.Rest()
	if s.IsEmpty() {
		return nil, nil, false
	}
	return k, s.First(), true
}
func IsTransientObject(obj coretypes.Object) bool {
	switch obj.(type) {
	case *coretypes.TransientVector, *coretypes.TransientMap:
		return true
	default:
		return false
	}
}
