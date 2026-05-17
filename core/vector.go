package core

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	corecollections "github.com/rcarmo/go-joker/core/collections"
)

type (
	Vec interface {
		coretypes.Object
		coretypes.CountedIndexed
		coretypes.Gettable
		Associative
		coretypes.Sequential
		coretypes.Comparable
		coretypes.Indexed
		coretypes.Stack
		Reversible
		Meta
		Seqable
		coretypes.Formatter
		coretypes.Callable
	}
	Vector struct {
		coretypes.InfoHolder
		MetaHolder
		root  []interface{}
		tail  []interface{}
		count int
		shift uint
	}
	VectorSeq struct {
		coretypes.InfoHolder
		MetaHolder
		vector coretypes.CountedIndexed
		index  int
	}
	VectorRSeq struct {
		coretypes.InfoHolder
		MetaHolder
		vector coretypes.CountedIndexed
		index  int
	}
)

var empty_node = make([]interface{}, 32)

func (v *Vector) WithMeta(meta Map) coretypes.Object {
	res := *v
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func clone(s []interface{}) []interface{} {
	return corecollections.CloneSlice(s)
}

func (v *Vector) tailoff() int {
	if v.count < 32 {
		return 0
	}
	return ((v.count - 1) >> 5) << 5
}

func (v *Vector) arrayFor(i int) []interface{} {
	if i >= v.tailoff() {
		return v.tail
	}
	node := v.root
	for level := v.shift; level > 0; level -= 5 {
		node = node[(i>>level)&0x01F].([]interface{})
	}
	return node
}

func (v *Vector) at(i int) coretypes.Object {
	if i >= v.count || i < 0 {
		panic(RT.NewError(fmt.Sprintf("Index %d is out of bounds [0..%d]", i, v.count-1)))
	}
	return v.arrayFor(i)[i&0x01F].(coretypes.Object)
}

func (v *Vector) uncheckedAt(i int) coretypes.Object {
	return v.arrayFor(i)[i&0x01F].(coretypes.Object)
}

func (v *Vector) At(i int) coretypes.Object {
	return v.uncheckedAt(i)
}

func newPath(level uint, node []interface{}) []interface{} {
	if level == 0 {
		return node
	}
	result := make([]interface{}, 32)
	result[0] = newPath(level-5, node)
	return result
}

func (v *Vector) pushTail(level uint, parent []interface{}, tailNode []interface{}) []interface{} {
	subidx := ((v.count - 1) >> level) & 0x01F
	result := clone(parent)
	var nodeToInsert []interface{}
	if level == 5 {
		nodeToInsert = tailNode
	} else {
		if parent[subidx] != nil {
			nodeToInsert = v.pushTail(level-5, parent[subidx].([]interface{}), tailNode)
		} else {
			nodeToInsert = newPath(level-5, tailNode)
		}
	}
	result[subidx] = nodeToInsert
	return result
}

func (v *Vector) Conjoin(obj coretypes.Object) *Vector {
	var newTail []interface{}
	if v.count-v.tailoff() < 32 {
		newTail = append(clone(v.tail), obj)
		return &Vector{count: v.count + 1, shift: v.shift, root: v.root, tail: newTail}
	}
	var newRoot []interface{}
	newShift := v.shift
	if (v.count >> 5) > (1 << v.shift) {
		newRoot = make([]interface{}, 32)
		newRoot[0] = v.root
		newRoot[1] = newPath(v.shift, v.tail)
		newShift += 5
	} else {
		newRoot = v.pushTail(v.shift, v.root, v.tail)
	}
	newTail = make([]interface{}, 1, 32)
	newTail[0] = obj
	return &Vector{count: v.count + 1, shift: newShift, root: newRoot, tail: newTail}
}

func (v *Vector) ToString(escape bool) string {
	return CountedIndexedToString(v, escape)
}

func (v *Vector) Equals(other interface{}) bool {
	if v == other {
		return true
	}
	switch other := other.(type) {
	case coretypes.CountedIndexed:
		return AreCountedIndexedEqual(v, other)
	default:
		return IsSeqEqual(v.Seq(), other)
	}
}

func (v *Vector) GetType() *coretypes.Type {
	return TYPE.Vector
}

func (v *Vector) Hash() uint32 {
	return CountedIndexedHash(v)
}

func (seq *VectorSeq) Seq() Seq {
	return seq
}

func (seq *VectorSeq) Equals(other interface{}) bool {
	return IsSeqEqual(seq, other)
}

func (seq *VectorSeq) ToString(escape bool) string {
	return SeqToString(seq, escape)
}

func (seq *VectorSeq) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *VectorSeq) Format(w io.Writer, indent int) int {
	return formatSeq(seq, w, indent)
}

func (seq *VectorSeq) WithMeta(meta Map) coretypes.Object {
	res := *seq
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func (seq *VectorSeq) GetType() *coretypes.Type {
	return TYPE.VectorSeq
}

func (seq *VectorSeq) Hash() uint32 {
	return hashOrdered(seq)
}

func (seq *VectorSeq) First() coretypes.Object {
	if seq.index < seq.vector.Count() {
		return seq.vector.At(seq.index)
	}
	return NIL
}

func (seq *VectorSeq) Rest() Seq {
	if seq.index+1 < seq.vector.Count() {
		return &VectorSeq{vector: seq.vector, index: seq.index + 1}
	}
	return EmptyList
}

func (seq *VectorSeq) IsEmpty() bool {
	return seq.index >= seq.vector.Count()
}

func (seq *VectorSeq) Count() int {
	n := seq.vector.Count() - seq.index
	if n < 0 {
		return 0
	}
	return n
}

func (seq *VectorSeq) Cons(obj coretypes.Object) Seq {
	return &ConsSeq{first: obj, rest: seq}
}

func (seq *VectorSeq) SequentialMarker() {}

func (seq *VectorRSeq) Seq() Seq {
	return seq
}

func (seq *VectorRSeq) Equals(other interface{}) bool {
	return IsSeqEqual(seq, other)
}

func (seq *VectorRSeq) ToString(escape bool) string {
	return SeqToString(seq, escape)
}

func (seq *VectorRSeq) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *VectorRSeq) Format(w io.Writer, indent int) int {
	return formatSeq(seq, w, indent)
}

func (seq *VectorRSeq) WithMeta(meta Map) coretypes.Object {
	res := *seq
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func (seq *VectorRSeq) GetType() *coretypes.Type {
	return TYPE.VectorRSeq
}

func (seq *VectorRSeq) Hash() uint32 {
	return hashOrdered(seq)
}

func (seq *VectorRSeq) First() coretypes.Object {
	if seq.index >= 0 {
		return seq.vector.At(seq.index)
	}
	return NIL
}

func (seq *VectorRSeq) Rest() Seq {
	if seq.index-1 >= 0 {
		return &VectorRSeq{vector: seq.vector, index: seq.index - 1}
	}
	return EmptyList
}

func (seq *VectorRSeq) IsEmpty() bool {
	return seq.index < 0
}

func (seq *VectorRSeq) Count() int {
	if seq.index < 0 {
		return 0
	}
	return seq.index + 1
}

func (seq *VectorRSeq) Cons(obj coretypes.Object) Seq {
	return &ConsSeq{first: obj, rest: seq}
}

func (seq *VectorRSeq) SequentialMarker() {}

func (v *Vector) Seq() Seq {
	return &VectorSeq{vector: v, index: 0}
}

func (v *Vector) Conj(obj coretypes.Object) coretypes.Conjable {
	return v.Conjoin(obj)
}

func (v *Vector) Count() int {
	return v.count
}

func (v *Vector) Nth(i int) coretypes.Object {
	return v.at(i)
}

func (v *Vector) TryNth(i int, d coretypes.Object) coretypes.Object {
	if i < 0 || i >= v.count {
		return d
	}
	return v.at(i)
}

func (v *Vector) SequentialMarker() {}

func (v *Vector) Compare(other coretypes.Object) int {
	v2 := EnsureObjectIsCountedIndexed(rootObject(other), "Cannot compare Vector: %s")
	return CountedIndexedCompare(v, v2)
}

func (v *Vector) Peek() coretypes.Object {
	if v.count > 0 {
		return v.Nth(v.count - 1)
	}
	return NIL
}

func (v *Vector) popTail(level uint, node []interface{}) []interface{} {
	subidx := ((v.count - 2) >> level) & 0x01F
	if level > 5 {
		newChild := v.popTail(level-5, node[subidx].([]interface{}))
		if newChild == nil && subidx == 0 {
			return nil
		} else {
			ret := clone(node)
			ret[subidx] = newChild
			return ret
		}
	} else if subidx == 0 {
		return nil
	} else {
		ret := clone(node)
		ret[subidx] = nil
		return ret
	}
}

func (v *Vector) Pop() coretypes.Stack {
	if v.count == 0 {
		panic(RT.NewError("Can't pop empty vector"))
	}
	if v.count == 1 {
		return collectionConstruction.NewEmptyVector().WithMeta(v.meta).(coretypes.Stack)
	}
	if v.count-v.tailoff() > 1 {
		newTail := clone(v.tail)[0 : len(v.tail)-1]
		res := &Vector{count: v.count - 1, shift: v.shift, root: v.root, tail: newTail}
		res.meta = v.meta
		return res
	}
	newTail := v.arrayFor(v.count - 2)
	newRoot := v.popTail(v.shift, v.root)
	newShift := v.shift
	if newRoot == nil {
		newRoot = empty_node
	}
	if v.shift > 5 && newRoot[1] == nil {
		newRoot = newRoot[0].([]interface{})
		newShift -= 5
	}
	res := &Vector{count: v.count - 1, shift: newShift, root: newRoot, tail: newTail}
	res.meta = v.meta
	return res
}

func (v *Vector) Get(key coretypes.Object) (bool, coretypes.Object) {
	return CountedIndexedGet(v, key)
}

func (v *Vector) EntryAt(key coretypes.Object) *ArrayVector {
	ok, val := v.Get(key)
	if ok {
		return collectionConstruction.NewArrayVectorFrom(key, val)
	}
	return nil
}

func doAssoc(level uint, node []interface{}, i int, val coretypes.Object) []interface{} {
	ret := clone(node)
	if level == 0 {
		ret[i&0x01f] = val
	} else {
		subidx := (i >> level) & 0x01f
		ret[subidx] = doAssoc(level-5, node[subidx].([]interface{}), i, val)
	}
	return ret
}

func (v *Vector) assocN(i int, val coretypes.Object) *Vector {
	if i < 0 || i > v.count {
		panic(RT.NewError((fmt.Sprintf("Index %d is out of bounds [0..%d]", i, v.count))))
	}
	if i == v.count {
		return v.Conjoin(val)
	}
	if i < v.tailoff() {
		res := &Vector{count: v.count, shift: v.shift, root: doAssoc(v.shift, v.root, i, val), tail: v.tail}
		res.meta = v.meta
		return res
	}
	newTail := clone(v.tail)
	newTail[i&0x01f] = val
	res := &Vector{count: v.count, shift: v.shift, root: v.root, tail: newTail}
	res.meta = v.meta
	return res
}

func assertInteger(obj coretypes.Object) int {
	var i int
	switch obj := obj.(type) {
	case coretypes.Int:
		i = obj.I
	case *coretypes.BigInt:
		i = obj.Int().I
	default:
		panic(RT.NewError("Key must be integer"))
	}
	return i
}

func (v *Vector) Assoc(key, val coretypes.Object) Associative {
	i := assertInteger(key)
	return v.assocN(i, val)
}

func (v *Vector) Rseq() Seq {
	return &VectorRSeq{vector: v, index: v.count - 1}
}

func (v *Vector) Call(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	i := assertInteger(args[0])
	return v.at(i)
}

func EmptyVector() *Vector {
	return &Vector{
		count: 0,
		shift: 5,
		root:  empty_node,
		tail:  make([]interface{}, 0, 32),
	}
}

func NewVectorFrom(objs ...coretypes.Object) *Vector {
	n := len(objs)
	if n == 0 {
		return EmptyVector()
	}
	if n <= 32 {
		tail := make([]interface{}, n)
		for i, o := range objs {
			tail[i] = o
		}
		return &Vector{count: n, shift: 5, root: empty_node, tail: tail}
	}
	// First 32 in one tail, then Conjoin the rest.
	tail := make([]interface{}, 32)
	for i := 0; i < 32; i++ {
		tail[i] = objs[i]
	}
	res := &Vector{count: 32, shift: 5, root: empty_node, tail: tail}
	for i := 32; i < n; i++ {
		res = res.Conjoin(objs[i])
	}
	return res
}

func NewVectorFromSeq(seq Seq) *Vector {
	if c, ok := seq.(coretypes.Counted); ok {
		n := c.Count()
		if n == 0 {
			return EmptyVector()
		}
		objs := make([]coretypes.Object, n)
		for i := 0; i < n; i++ {
			objs[i] = seq.First()
			seq = seq.Rest()
		}
		return NewVectorFrom(objs...)
	}
	res := EmptyVector()
	for !seq.IsEmpty() {
		res = res.Conjoin(seq.First())
		seq = seq.Rest()
	}
	return res
}

func (v *Vector) Empty() Collection {
	return collectionConstruction.NewEmptyVector()
}

func (v *Vector) KVReduce(c coretypes.Callable, init coretypes.Object) coretypes.Object {
	return CountedIndexedKvreduce(v, c, init)
}

func (v *Vector) Pprint(w io.Writer, indent int) int {
	return CountedIndexedPprint(v, w, indent)
}

func (v *Vector) Format(w io.Writer, indent int) int {
	return CountedIndexedFormat(v, w, indent)
}

func (v *Vector) Reduce(c coretypes.Callable) coretypes.Object {
	return CountedIndexedReduce(v, c)
}

func (v *Vector) ReduceInit(c coretypes.Callable, init coretypes.Object) coretypes.Object {
	return CountedIndexedReduceInit(v, c, init)
}
