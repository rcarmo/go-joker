package collections

import (
	"fmt"
	"io"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type (
	ArrayMap struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		Arr []coretypes.Object
	}
	ArrayMapIterator struct {
		M       *ArrayMap
		Current int
	}
	ArrayMapSeq struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		M     *ArrayMap
		Index int
	}
)

var (
	HASHMAP_THRESHOLD int64 = 16
)

func EmptyArrayMap() *ArrayMap {
	return &ArrayMap{}
}

func ArraySeqFromArrayMap(m *ArrayMap) *ArraySeq {
	return &ArraySeq{Arr: m.Arr}
}

func (seq *ArrayMapSeq) SequentialMarker() {}

func (seq *ArrayMapSeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(seq, other)
}

func (seq *ArrayMapSeq) ToString(escape bool) string {
	return SeqToString(seq, func(obj coretypes.Object) string { return obj.ToString(escape) })
}

func (seq *ArrayMapSeq) Pprint(w io.Writer, indent int) int {
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

func (seq *ArrayMapSeq) Format(w io.Writer, indent int) int {
	return seq.Pprint(w, indent)
}

func (seq *ArrayMapSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	seq.Info = info
	return seq
}

func (seq *ArrayMapSeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *seq
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (seq *ArrayMapSeq) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.ArrayMapSeq
}

func (seq *ArrayMapSeq) Hash() uint32 {
	return HashOrdered(seq)
}

func (seq *ArrayMapSeq) Seq() coretypes.Seq {
	return seq
}

func (seq *ArrayMapSeq) First() coretypes.Object {
	if key, value, ok := PairAt(seq.M.Arr, seq.Index); ok {
		return NewVectorFrom(key, value)
	}
	return coretypes.RuntimeNil
}

func (seq *ArrayMapSeq) Rest() coretypes.Seq {
	if !PairIndexEmpty(seq.Index, len(seq.M.Arr)) {
		return &ArrayMapSeq{M: seq.M, Index: NextPairIndex(seq.Index)}
	}
	return EmptyList
}

func (seq *ArrayMapSeq) IsEmpty() bool {
	return PairIndexEmpty(seq.Index, len(seq.M.Arr))
}

func (seq *ArrayMapSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &ConsSeq{FirstValue: obj, RestValue: seq}
}

func (iter *ArrayMapIterator) Next() *coretypes.Pair {
	key, value, next := IteratorNextPair(iter.M.Arr, iter.Current)
	iter.Current = next
	return &coretypes.Pair{Key: key, Value: value}
}

func (iter *ArrayMapIterator) HasNext() bool {
	return IteratorHasNext(iter.Current, len(iter.M.Arr))
}

func (v *ArrayMap) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	v.Info = info
	return v
}

func WithMergedMeta[T any](v T, current coretypes.Map, meta coretypes.Map, set func(*T, coretypes.Map)) T {
	res := v
	set(&res, coretypes.SafeMerge(current, meta))
	return res
}

func (v *ArrayMap) WithMeta(meta coretypes.Map) coretypes.Object {
	res := WithMergedMeta(*v, v.Meta, meta, func(am *ArrayMap, m coretypes.Map) { am.Meta = m })
	return &res
}

func (m *ArrayMap) IndexOf(key coretypes.Object) int {
	return MapIndexOf(m.Arr, key)
}

func (m *ArrayMap) ArrayItems() []coretypes.Object {
	return m.Arr
}

func (m *ArrayMap) Get(key coretypes.Object) (bool, coretypes.Object) {
	found, value := MapGet(m.Arr, key)
	if found {
		return true, value
	}
	return false, nil
}

func (m *ArrayMap) Set(key coretypes.Object, value coretypes.Object) {
	m.Arr = ArrayMapSet(m.Arr, key, value)
}

func (m *ArrayMap) Add(key coretypes.Object, value coretypes.Object) bool {
	var ok bool
	m.Arr, ok = MapAdd(m.Arr, key, value)
	return ok
}

func (m *ArrayMap) Plus(key coretypes.Object, value coretypes.Object) *ArrayMap {
	if exists, _ := MapGet(m.Arr, key); exists {
		return m
	}
	m.Arr = ArrayMapSet(m.Arr, key, value)
	return m
}

func (m *ArrayMap) Count() int {
	return ArrayMapCount(m.Arr)
}

func (m *ArrayMap) Clone() *ArrayMap {
	return &ArrayMap{MetaHolder: m.MetaHolder, Arr: CloneSlice(m.Arr)}
}

func (m *ArrayMap) Assoc(key coretypes.Object, value coretypes.Object) coretypes.Associative {
	next, useHash := MapAssoc(m.Arr, key, value, HASHMAP_THRESHOLD)
	if useHash {
		return NewHashMap(m.Arr...).Assoc(key, value)
	}
	return &ArrayMap{MetaHolder: m.MetaHolder, Arr: next}
}

func (m *ArrayMap) EntryAt(key coretypes.Object) coretypes.Object {
	if value, ok := MapEntryAt(m.Arr, key); ok {
		return NewArrayVectorFrom(key, value)
	}
	return nil
}

func (m *ArrayMap) Without(key coretypes.Object) coretypes.Map {
	return &ArrayMap{Arr: MapWithout(m.Arr, key)}
}

func (m *ArrayMap) Merge(other coretypes.Map) coretypes.Map {
	if other.Count() == 0 {
		return m
	}
	if m.Count() == 0 {
		return other
	}
	pairs := make([]coretypes.Pair, 0, other.Count())
	for iter := other.Iter(); iter.HasNext(); {
		pairs = append(pairs, *iter.Next())
	}
	next, useHash := MapMergePairs(m.Arr, pairs, HASHMAP_THRESHOLD)
	if useHash {
		return NewHashMap(m.Arr...).Merge(other)
	}
	return &ArrayMap{MetaHolder: m.MetaHolder, Arr: next}
}

func (m *ArrayMap) Keys() coretypes.Seq {
	return &ArraySeq{Arr: MapKeys(m.Arr)}
}

func (m *ArrayMap) Vals() coretypes.Seq {
	return &ArraySeq{Arr: MapVals(m.Arr)}
}

func (m *ArrayMap) Iter() coretypes.MapIterator {
	return &ArrayMapIterator{M: m}
}

func (m *ArrayMap) Conj(obj coretypes.Object) coretypes.Conjable {
	return MapConj(m, obj, func(msg string) any { return coretypes.RuntimeError(msg) })
}

func (m *ArrayMap) ToString(escape bool) string {
	return MapToString(m, escape)
}

func (m *ArrayMap) Equals(other interface{}) bool {
	return MapEquals(m, other)
}

func (m *ArrayMap) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.ArrayMap
}

func (m *ArrayMap) Hash() uint32 {
	return HashUnordered(m.Seq(), 1)
}

func (m *ArrayMap) Seq() coretypes.Seq {
	return &ArrayMapSeq{M: m, Index: 0}
}

func (m *ArrayMap) Call(args []coretypes.Object) coretypes.Object {
	return CallMap(m, args, func(args []coretypes.Object, min int, max int) {
		if len(args) < min || len(args) > max {
			coretypes.RuntimePanicArityMinMax(len(args), min, max)
		}
	}, coretypes.RuntimeNil)
}

func (m *ArrayMap) Empty() coretypes.Collection {
	return EmptyArrayMap()
}

func (m *ArrayMap) Pprint(w io.Writer, indent int) int {
	return PprintMap(m, w, indent, coretypes.RuntimePprintObject, coretypes.RuntimeWriteIndent)
}

func (m *ArrayMap) Format(w io.Writer, indent int) int {
	return ArrayMapFormat[coretypes.Object](m.Arr, w, indent, coretypes.RuntimeFormatObject, coretypes.RuntimeMaybeNewLine, coretypes.RuntimeIsComment, coretypes.RuntimeWriteIndent)
}
