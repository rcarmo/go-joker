package collections

import (
	"fmt"
	"io"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type (
	Vector struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		Root       []interface{}
		Tail       []interface{}
		CountValue int
		Shift      uint
	}
	VectorSeq struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		Vector coretypes.CountedIndexed
		Index  int
	}
	VectorRSeq struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		Vector coretypes.CountedIndexed
		Index  int
	}

	VectorConsSeq struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		FirstValue coretypes.Object
		RestValue  coretypes.Seq
	}
)

var empty_node = make([]interface{}, 32)

func (v *Vector) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	v.Info = info
	return v
}

func (v *Vector) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *v
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (v *Vector) tailoff() int {
	return VectorTailOffset(v.CountValue)
}

func (v *Vector) arrayFor(i int) []interface{} {
	return VectorArrayFor(i, v.CountValue, v.Shift, v.Root, v.Tail)
}

func (v *Vector) at(i int) coretypes.Object {
	if i >= v.CountValue || i < 0 {
		panic(coretypes.RuntimeError(fmt.Sprintf("Index %d is out of bounds [0..%d]", i, v.CountValue-1)))
	}
	return v.arrayFor(i)[i&0x01F].(coretypes.Object)
}

func (v *Vector) uncheckedAt(i int) coretypes.Object {
	if value, ok := InterfaceNth(v.arrayFor(i), i&0x01F); ok {
		return value
	}
	return coretypes.RuntimeNil
}

func (v *Vector) At(i int) coretypes.Object {
	return v.uncheckedAt(i)
}

func (v *Vector) pushTail(level uint, parent []interface{}, tailNode []interface{}) []interface{} {
	return VectorPushTail(level, v.CountValue, parent, tailNode)
}

func (v *Vector) Conjoin(obj coretypes.Object) *Vector {
	newCount, newShift, newRoot, newTail := VectorConjoinState(v.CountValue, v.Shift, v.Root, v.Tail, obj, VectorTailOffset, func(level uint, parent []interface{}, tailNode []interface{}) []interface{} {
		return v.pushTail(level, parent, tailNode)
	})
	return &Vector{CountValue: newCount, Shift: newShift, Root: newRoot, Tail: newTail}
}

func (v *Vector) ToString(escape bool) string {
	return IndexedToString[coretypes.Object](v, escape)
}

func (v *Vector) Equals(other interface{}) bool {
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

func (v *Vector) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.Vector
}

func (v *Vector) Hash() uint32 {
	return IndexedHash[coretypes.Object](v)
}

func (seq *VectorSeq) Seq() coretypes.Seq {
	return seq
}

func (seq *VectorSeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(seq, other)
}

func (seq *VectorSeq) ToString(escape bool) string {
	return SeqToString(seq, func(obj coretypes.Object) string { return obj.ToString(escape) })
}

func (seq *VectorSeq) Pprint(w io.Writer, indent int) int {
	return seqPprint(seq, w, indent)
}

func (seq *VectorSeq) Format(w io.Writer, indent int) int {
	return seqFormat(seq, w, indent)
}

func (seq *VectorSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	seq.Info = info
	return seq
}

func (seq *VectorSeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *seq
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (seq *VectorSeq) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.VectorSeq
}

func (seq *VectorSeq) Hash() uint32 {
	return HashOrdered(seq)
}

func (seq *VectorSeq) First() coretypes.Object {
	if value, ok := SeqFirst(seq.Index, seq.Vector.Count(), seq.Vector.At); ok {
		return value
	}
	return coretypes.RuntimeNil
}

func (seq *VectorSeq) Rest() coretypes.Seq {
	if next, ok := SeqRestIndex(seq.Index, seq.Vector.Count()); ok {
		return &VectorSeq{Vector: seq.Vector, Index: next}
	}
	return &VectorSeq{Vector: seq.Vector, Index: seq.Vector.Count()}
}

func (seq *VectorSeq) IsEmpty() bool {
	return SeqIsEmpty(seq.Index, seq.Vector.Count())
}

func (seq *VectorSeq) Count() int {
	return SeqIndexCount(seq.Index, seq.Vector.Count())
}

func (seq *VectorSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &VectorConsSeq{FirstValue: obj, RestValue: seq}
}

func (seq *VectorSeq) SequentialMarker() {}

func (seq *VectorRSeq) Seq() coretypes.Seq {
	return seq
}

func (seq *VectorRSeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(seq, other)
}

func (seq *VectorRSeq) ToString(escape bool) string {
	return SeqToString(seq, func(obj coretypes.Object) string { return obj.ToString(escape) })
}

func (seq *VectorRSeq) Pprint(w io.Writer, indent int) int {
	return seqPprint(seq, w, indent)
}

func (seq *VectorRSeq) Format(w io.Writer, indent int) int {
	return seqFormat(seq, w, indent)
}

func (seq *VectorRSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	seq.Info = info
	return seq
}

func (seq *VectorRSeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *seq
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (seq *VectorRSeq) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.VectorRSeq
}

func (seq *VectorRSeq) Hash() uint32 {
	return HashOrdered(seq)
}

func (seq *VectorRSeq) First() coretypes.Object {
	if value, ok := RSeqFirst(seq.Index, seq.Vector.At); ok {
		return value
	}
	return coretypes.RuntimeNil
}

func (seq *VectorRSeq) Rest() coretypes.Seq {
	if next, ok := RSeqRestIndex(seq.Index); ok {
		return &VectorRSeq{Vector: seq.Vector, Index: next}
	}
	return &VectorSeq{Vector: seq.Vector, Index: seq.Vector.Count()}
}

func (seq *VectorRSeq) IsEmpty() bool {
	return RSeqIsEmpty(seq.Index)
}

func (seq *VectorRSeq) Count() int {
	return RSeqCount(seq.Index)
}

func (seq *VectorRSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &VectorConsSeq{FirstValue: obj, RestValue: seq}
}

func (seq *VectorRSeq) SequentialMarker() {}

func (v *Vector) Seq() coretypes.Seq {
	return &VectorSeq{Vector: v, Index: 0}
}

func (v *Vector) Conj(obj coretypes.Object) coretypes.Conjable {
	return v.Conjoin(obj)
}

func (v *Vector) Count() int {
	return v.CountValue
}

func (v *Vector) Nth(i int) coretypes.Object {
	return v.at(i)
}

func (v *Vector) TryNth(i int, d coretypes.Object) coretypes.Object {
	if i < 0 || i >= v.CountValue {
		return d
	}
	return v.at(i)
}

func (v *Vector) SequentialMarker() {}

func (v *Vector) Compare(other coretypes.Object) int {
	v2 := coretypes.EnsureObjectIsCountedIndexed(coretypes.RootObject(other), "Cannot compare coretypes.Vector: %s")
	return IndexedCompare[coretypes.Object](v, v2, func(a, b coretypes.Object) int { return coretypes.EnsureObjectIsComparable(a, "").Compare(b) })
}

func (v *Vector) Peek() coretypes.Object {
	if v.CountValue > 0 {
		return v.Nth(v.CountValue - 1)
	}
	return coretypes.RuntimeNil
}

func (v *Vector) popTail(level uint, node []interface{}) []interface{} {
	return VectorPopTail(level, v.CountValue, node)
}

func (v *Vector) Pop() coretypes.Stack {
	if v.CountValue == 0 {
		panic(coretypes.RuntimeError("Can't pop empty vector"))
	}
	if v.CountValue == 1 {
		return EmptyVector().WithMeta(v.Meta).(coretypes.Stack)
	}
	if v.CountValue-v.tailoff() > 1 {
		newTail := CloneSlice(v.Tail)[0 : len(v.Tail)-1]
		res := &Vector{CountValue: v.CountValue - 1, Shift: v.Shift, Root: v.Root, Tail: newTail}
		res.Meta = v.Meta
		return res
	}
	newTail := v.arrayFor(v.CountValue - 2)
	newRoot := v.popTail(v.Shift, v.Root)
	newShift := v.Shift
	if newRoot == nil {
		newRoot = empty_node
	}
	if v.Shift > 5 && newRoot[1] == nil {
		newRoot = newRoot[0].([]interface{})
		newShift -= 5
	}
	res := &Vector{CountValue: v.CountValue - 1, Shift: newShift, Root: newRoot, Tail: newTail}
	res.Meta = v.Meta
	return res
}

func (v *Vector) Get(key coretypes.Object) (bool, coretypes.Object) {
	return IndexedGetByObject[coretypes.Object](v, key)
}

func (v *Vector) EntryAt(key coretypes.Object) coretypes.Object {
	ok, val := v.Get(key)
	if ok {
		return NewArrayVectorFrom(key, val)
	}
	return nil
}

func (v *Vector) assocN(i int, val coretypes.Object) *Vector {
	valid, appendMode, inRoot := VectorAssocState(i, v.CountValue, v.tailoff())
	if !valid {
		panic(coretypes.RuntimeError((fmt.Sprintf("Index %d is out of bounds [0..%d]", i, v.CountValue))))
	}
	if appendMode {
		return v.Conjoin(val)
	}
	if inRoot {
		res := &Vector{CountValue: v.CountValue, Shift: v.Shift, Root: VectorAssocNode(v.Shift, v.Root, i, val), Tail: v.Tail}
		res.Meta = v.Meta
		return res
	}
	newTail := CloneSlice(v.Tail)
	newTail[i&0x01f] = val
	res := &Vector{CountValue: v.CountValue, Shift: v.Shift, Root: v.Root, Tail: newTail}
	res.Meta = v.Meta
	return res
}

func (v *Vector) Assoc(key, val coretypes.Object) coretypes.Associative {
	i, ok := IndexFromObject(key)
	if !ok {
		panic(coretypes.RuntimeError("Key must be integer"))
	}
	return v.assocN(i, val)
}

func (v *Vector) Rseq() coretypes.Seq {
	return &VectorRSeq{Vector: v, Index: v.CountValue - 1}
}

func (v *Vector) Call(args []coretypes.Object) coretypes.Object {
	if len(args) != 1 {
		coretypes.RuntimePanicArityMinMax(len(args), 1, 1)
	}
	i, ok := IndexFromObject(args[0])
	if !ok {
		panic(coretypes.RuntimeError("Key must be integer"))
	}
	return v.at(i)
}

func EmptyVector() *Vector {
	return &Vector{
		CountValue: 0,
		Shift:      5,
		Root:       empty_node,
		Tail:       make([]interface{}, 0, 32),
	}
}

func NewVectorFrom(objs ...coretypes.Object) *Vector {
	n := len(objs)
	if n == 0 {
		return EmptyVector()
	}
	if n <= 32 {
		tail := InterfacesFromValues(objs)
		return &Vector{CountValue: n, Shift: 5, Root: empty_node, Tail: tail}
	}
	// First 32 in one tail, then Conjoin the rest.
	tail := make([]interface{}, 32)
	for i := 0; i < 32; i++ {
		tail[i] = objs[i]
	}
	res := &Vector{CountValue: 32, Shift: 5, Root: empty_node, Tail: tail}
	for i := 32; i < n; i++ {
		res = res.Conjoin(objs[i])
	}
	return res
}

func NewVectorFromSeq(seq coretypes.Seq) *Vector {
	if c, ok := seq.(coretypes.Counted); ok {
		n := c.Count()
		if n == 0 {
			return EmptyVector()
		}
		return NewVectorFrom(SeqToSliceN(seq, n)...)
	}
	return NewVectorFrom(SeqToSlice(seq)...)
}

func (v *Vector) Empty() coretypes.Collection {
	return EmptyVector()
}

func (v *Vector) KVReduce(c coretypes.Callable, init coretypes.Object) coretypes.Object {
	return IndexedKVReduce[coretypes.Object](v, init, func(res coretypes.Object, i int, value coretypes.Object) coretypes.Object {
		return c.Call([]coretypes.Object{res, coretypes.Int{I: i}, value})
	})
}

func (v *Vector) Pprint(w io.Writer, indent int) int {
	return IndexedPprint[coretypes.Object](v, w, indent, coretypes.RuntimePprintObject, coretypes.RuntimeWriteIndent)
}

func (v *Vector) Format(w io.Writer, indent int) int {
	return IndexedFormat[coretypes.Object](v, w, indent, coretypes.RuntimeFormatObject, coretypes.RuntimeMaybeNewLine, coretypes.RuntimeIsComment, coretypes.RuntimeWriteIndent)
}

func (v *Vector) Reduce(c coretypes.Callable) coretypes.Object {
	return IndexedReduce[coretypes.Object](v, func() coretypes.Object { return c.Call(nil) }, func(acc coretypes.Object, value coretypes.Object) coretypes.Object {
		return c.Call([]coretypes.Object{acc, value})
	})
}

func (v *Vector) ReduceInit(c coretypes.Callable, init coretypes.Object) coretypes.Object {
	return IndexedReduceInit[coretypes.Object](v, init, func(acc coretypes.Object, value coretypes.Object) coretypes.Object {
		return c.Call([]coretypes.Object{acc, value})
	})
}

func (seq *VectorConsSeq) Seq() coretypes.Seq            { return seq }
func (seq *VectorConsSeq) Equals(other interface{}) bool { return coretypes.IsSeqEqual(seq, other) }
func (seq *VectorConsSeq) ToString(escape bool) string {
	return SeqToString(seq, func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (seq *VectorConsSeq) Pprint(w io.Writer, indent int) int { return seqPprint(seq, w, indent) }
func (seq *VectorConsSeq) Format(w io.Writer, indent int) int { return seqFormat(seq, w, indent) }
func (seq *VectorConsSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	seq.Info = info
	return seq
}
func (seq *VectorConsSeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *seq
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}
func (seq *VectorConsSeq) GetType() *coretypes.Type { return coretypes.RuntimeTypes.ConsSeq }
func (seq *VectorConsSeq) Hash() uint32             { return HashOrdered(seq) }
func (seq *VectorConsSeq) First() coretypes.Object  { return seq.FirstValue }
func (seq *VectorConsSeq) Rest() coretypes.Seq      { return seq.RestValue }
func (seq *VectorConsSeq) IsEmpty() bool            { return false }
func (seq *VectorConsSeq) Count() int {
	if counted, ok := seq.RestValue.(coretypes.Counted); ok {
		return 1 + counted.Count()
	}
	count := 1
	for rest := seq.RestValue; rest != nil && !rest.IsEmpty(); rest = rest.Rest() {
		count++
	}
	return count
}
func (seq *VectorConsSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &VectorConsSeq{FirstValue: obj, RestValue: seq}
}
func (seq *VectorConsSeq) SequentialMarker() {}

func seqPprint(seq coretypes.Seq, w io.Writer, indent int) int {
	i := indent + 1
	fmt.Fprint(w, "(")
	for iter := NewSeqIterator(seq); iter.HasNext(); {
		i = coretypes.RuntimePprintObject(iter.Next(), indent+1, w)
		if iter.HasNext() {
			fmt.Fprint(w, "\n")
			coretypes.RuntimeWriteIndent(w, indent+1)
		}
	}
	fmt.Fprint(w, ")")
	return i + 1
}

func seqFormat(seq coretypes.Seq, w io.Writer, indent int) int {
	return seqPprint(seq, w, indent)
}
