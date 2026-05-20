package collections

import (
	"fmt"
	"io"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type (
	ConsSeq struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		FirstValue coretypes.Object
		RestValue  coretypes.Seq
	}
	ArraySeq struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		Arr   []coretypes.Object
		Index int
	}
	LazySeq struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		Fn       coretypes.Callable
		SeqValue coretypes.Seq
	}
	MappingSeq struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		SeqValue coretypes.Seq
		Fn       func(obj coretypes.Object) coretypes.Object
	}
)

func (seq *MappingSeq) Seq() coretypes.Seq {
	return seq
}

func (seq *MappingSeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(seq, other)
}

func (seq *MappingSeq) ToString(escape bool) string {
	return SeqToString(seq, func(obj coretypes.Object) string { return obj.ToString(escape) })
}

func (seq *MappingSeq) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *MappingSeq) Format(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *MappingSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	seq.Info = info
	return seq
}

func (seq *MappingSeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *seq
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (seq *MappingSeq) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.MappingSeq
}

func (seq *MappingSeq) Hash() uint32 {
	return HashOrdered(seq)
}

func (seq *MappingSeq) First() coretypes.Object {
	return seq.Fn(seq.SeqValue.First())
}

func (seq *MappingSeq) Rest() coretypes.Seq {
	return &MappingSeq{
		SeqValue: seq.SeqValue.Rest(),
		Fn:       seq.Fn,
	}
}

func (seq *MappingSeq) IsEmpty() bool {
	return seq.SeqValue.IsEmpty()
}

func (seq *MappingSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &ConsSeq{FirstValue: obj, RestValue: seq}
}

func (seq *MappingSeq) SequentialMarker() {}

func (seq *LazySeq) Seq() coretypes.Seq {
	return seq
}

func (seq *LazySeq) realize() {
	if seq.SeqValue == nil {
		seq.SeqValue = coretypes.EnsureObjectIsSeqable(seq.Fn.Call(nil), "").Seq()
	}
}

func (seq *LazySeq) IsRealized() bool {
	return seq.SeqValue != nil
}

func (seq *LazySeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(seq, other)
}

func (seq *LazySeq) ToString(escape bool) string {
	return SeqToString(seq, func(obj coretypes.Object) string { return obj.ToString(escape) })
}

func (seq *LazySeq) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *LazySeq) Format(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *LazySeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	seq.Info = info
	return seq
}

func (seq *LazySeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *seq
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (seq *LazySeq) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.LazySeq
}

func (seq *LazySeq) Hash() uint32 {
	return HashOrdered(seq)
}

func (seq *LazySeq) First() coretypes.Object {
	seq.realize()
	return seq.SeqValue.First()
}

func (seq *LazySeq) Rest() coretypes.Seq {
	seq.realize()
	return seq.SeqValue.Rest()
}

func (seq *LazySeq) IsEmpty() bool {
	seq.realize()
	return seq.SeqValue.IsEmpty()
}

func (seq *LazySeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &ConsSeq{FirstValue: obj, RestValue: seq}
}

func (seq *LazySeq) SequentialMarker() {}

func NewLazySeq(c coretypes.Callable) *LazySeq {
	return &LazySeq{Fn: c}
}

func (seq *ArraySeq) Seq() coretypes.Seq {
	return seq
}

func (seq *ArraySeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(seq, other)
}

func (seq *ArraySeq) ToString(escape bool) string {
	return SeqToString(seq, func(obj coretypes.Object) string { return obj.ToString(escape) })
}

func (seq *ArraySeq) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *ArraySeq) Format(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *ArraySeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	seq.Info = info
	return seq
}

func (seq *ArraySeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *seq
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (seq *ArraySeq) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.ArraySeq
}

func (seq *ArraySeq) Hash() uint32 {
	return HashOrdered(seq)
}

func (seq *ArraySeq) First() coretypes.Object {
	if v, ok := ArraySeqFirst(seq.Arr, seq.Index); ok {
		return v
	}
	return coretypes.RuntimeNil
}

func (seq *ArraySeq) Rest() coretypes.Seq {
	if next, ok := ArraySeqRest(seq.Index, len(seq.Arr)); ok {
		return &ArraySeq{Index: next, Arr: seq.Arr}
	}
	return EmptyList
}

func (seq *ArraySeq) IsEmpty() bool {
	return ArraySeqIsEmpty(seq.Index, len(seq.Arr))
}

func (seq *ArraySeq) Count() int {
	return ArraySeqCount(seq.Index, len(seq.Arr))
}

func (seq *ArraySeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &ConsSeq{FirstValue: obj, RestValue: seq}
}

func (seq *ArraySeq) SequentialMarker() {}

func (seq *ConsSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	seq.Info = info
	return seq
}

func (seq *ConsSeq) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *seq
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (seq *ConsSeq) Seq() coretypes.Seq {
	return seq
}

func (seq *ConsSeq) Equals(other interface{}) bool {
	return coretypes.IsSeqEqual(seq, other)
}

func (seq *ConsSeq) ToString(escape bool) string {
	return SeqToString(seq, func(obj coretypes.Object) string { return obj.ToString(escape) })
}

func (seq *ConsSeq) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *ConsSeq) Format(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *ConsSeq) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.ConsSeq
}

func (seq *ConsSeq) Hash() uint32 {
	return HashOrdered(seq)
}

func (seq *ConsSeq) First() coretypes.Object {
	return seq.FirstValue
}

func (seq *ConsSeq) Rest() coretypes.Seq {
	return seq.RestValue
}

func (seq *ConsSeq) IsEmpty() bool {
	return false
}

func (seq *ConsSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &ConsSeq{FirstValue: obj, RestValue: seq}
}

func (seq *ConsSeq) SequentialMarker() {}

func NewConsSeq(first coretypes.Object, rest coretypes.Seq) *ConsSeq {
	return &ConsSeq{FirstValue: first, RestValue: rest}
}

func iter(seq coretypes.Seq) *SeqIterator { return NewSeqIterator(seq) }

func Second(seq coretypes.Seq) coretypes.Object { return SeqSecond(seq) }
func Third(seq coretypes.Seq) coretypes.Object  { return SeqThird(seq) }
func Fourth(seq coretypes.Seq) coretypes.Object { return SeqFourth(seq) }

func ToSlice(seq coretypes.Seq) []coretypes.Object { return SeqToSlice(seq) }

func SeqNth(seq coretypes.Seq, n int) coretypes.Object {
	if n < 0 {
		panic(coretypes.RuntimeError(fmt.Sprintf("Negative index: %d", n)))
	}
	switch seq := seq.(type) {
	case *ArraySeq:
		if value, length, ok := SeqNthArray(seq.Arr, seq.Index, n); ok {
			return value
		} else {
			panic(coretypes.RuntimeError(SeqNthError(n, length)))
		}
	}
	if value, ok := SeqNthTry(seq, n); ok {
		return value
	}
	panic(coretypes.RuntimeError(SeqNthError(n, SeqCount(seq))))
}

func SeqTryNth(seq coretypes.Seq, n int, d coretypes.Object) coretypes.Object {
	if n < 0 {
		return d
	}
	switch seq := seq.(type) {
	case *ArraySeq:
		if value, _, ok := SeqNthArray(seq.Arr, seq.Index, n); ok {
			return value
		}
		return d
	}
	if value, ok := SeqNthTry(seq, n); ok {
		return value
	}
	return d
}

func hashUnordered(seq coretypes.Seq, seed uint32) uint32 { return HashUnordered(seq, seed) }
func hashOrdered(seq coretypes.Seq) uint32                { return HashOrdered(seq) }

func pprintSeq(seq coretypes.Seq, w io.Writer, indent int) int {
	i := indent + 1
	fmt.Fprint(w, "(")
	for iter := iter(seq); iter.HasNext(); {
		i = coretypes.RuntimePprintObject(iter.Next(), indent+1, w)
		if iter.HasNext() {
			fmt.Fprint(w, "\n")
			coretypes.RuntimeWriteIndent(w, indent+1)
		}
	}
	fmt.Fprint(w, ")")
	return i + 1
}

func SeqPprint(seq coretypes.Seq, w io.Writer, indent int) int { return pprintSeq(seq, w, indent) }

func seqReduceInit(s coretypes.Seq, f coretypes.Callable, init coretypes.Object) coretypes.Object {
	acc := init
	for !s.IsEmpty() {
		acc = f.Call([]coretypes.Object{acc, s.First()})
		if coretypes.RuntimeIsReduced != nil && coretypes.RuntimeIsReduced(acc) {
			return coretypes.RuntimeDerefReduced(acc)
		}
		s = s.Rest()
	}
	return acc
}

func seqReduce(s coretypes.Seq, f coretypes.Callable) coretypes.Object {
	if s.IsEmpty() {
		return f.Call(nil)
	}
	acc := s.First()
	s = s.Rest()
	for !s.IsEmpty() {
		acc = f.Call([]coretypes.Object{acc, s.First()})
		if coretypes.RuntimeIsReduced != nil && coretypes.RuntimeIsReduced(acc) {
			return coretypes.RuntimeDerefReduced(acc)
		}
		s = s.Rest()
	}
	return acc
}

func (seq *LazySeq) Reduce(f coretypes.Callable) coretypes.Object { return seqReduce(seq.Seq(), f) }
func (seq *LazySeq) ReduceInit(f coretypes.Callable, init coretypes.Object) coretypes.Object {
	return seqReduceInit(seq.Seq(), f, init)
}
func (seq *ConsSeq) Reduce(f coretypes.Callable) coretypes.Object { return seqReduce(seq, f) }
func (seq *ConsSeq) ReduceInit(f coretypes.Callable, init coretypes.Object) coretypes.Object {
	return seqReduceInit(seq, f, init)
}
func (seq *MappingSeq) Reduce(f coretypes.Callable) coretypes.Object {
	if seq.SeqValue.IsEmpty() {
		return f.Call(nil)
	}
	acc := seq.Fn(seq.SeqValue.First())
	cur := seq.SeqValue.Rest()
	for !cur.IsEmpty() {
		acc = f.Call([]coretypes.Object{acc, seq.Fn(cur.First())})
		if coretypes.RuntimeIsReduced != nil && coretypes.RuntimeIsReduced(acc) {
			return coretypes.RuntimeDerefReduced(acc)
		}
		cur = cur.Rest()
	}
	return acc
}
func (seq *MappingSeq) ReduceInit(f coretypes.Callable, init coretypes.Object) coretypes.Object {
	acc := init
	cur := seq.SeqValue
	for !cur.IsEmpty() {
		acc = f.Call([]coretypes.Object{acc, seq.Fn(cur.First())})
		if coretypes.RuntimeIsReduced != nil && coretypes.RuntimeIsReduced(acc) {
			return coretypes.RuntimeDerefReduced(acc)
		}
		cur = cur.Rest()
	}
	return acc
}
