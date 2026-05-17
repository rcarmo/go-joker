package core

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	"github.com/rcarmo/go-joker/core/bufferpool"
	"github.com/rcarmo/go-joker/core/hashutil"
)

type (
	Seq interface {
		Seqable
		coretypes.Object
		First() coretypes.Object
		Rest() Seq
		IsEmpty() bool
		Cons(obj coretypes.Object) Seq
	}
	Seqable interface {
		Seq() Seq
	}
	SeqIterator struct {
		seq Seq
	}
	ConsSeq struct {
		coretypes.InfoHolder
		MetaHolder
		first coretypes.Object
		rest  Seq
	}
	ArraySeq struct {
		coretypes.InfoHolder
		MetaHolder
		arr   []coretypes.Object
		index int
	}
	LazySeq struct {
		coretypes.InfoHolder
		MetaHolder
		fn  coretypes.Callable
		seq Seq
	}
	MappingSeq struct {
		coretypes.InfoHolder
		MetaHolder
		seq Seq
		fn  func(obj coretypes.Object) coretypes.Object
	}
)

func SeqsEqual(seq1, seq2 Seq) bool {
	iter2 := iter(seq2)
	for iter1 := iter(seq1); iter1.HasNext(); {
		if !iter2.HasNext() || !iter2.Next().Equals(iter1.Next()) {
			return false
		}
	}
	return !iter2.HasNext()
}

func IsSeqEqual(seq Seq, other interface{}) bool {
	if seq == other {
		return true
	}
	switch other := other.(type) {
	case coretypes.Sequential:
		switch other := other.(type) {
		case Seqable:
			return SeqsEqual(seq, other.Seq())
		}
	}
	return false
}

func (seq *MappingSeq) Seq() Seq {
	return seq
}

func (seq *MappingSeq) Equals(other interface{}) bool {
	return IsSeqEqual(seq, other)
}

func (seq *MappingSeq) ToString(escape bool) string {
	return SeqToString(seq, escape)
}

func (seq *MappingSeq) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *MappingSeq) Format(w io.Writer, indent int) int {
	return formatSeq(seq, w, indent)
}

func (seq *MappingSeq) WithMeta(meta Map) coretypes.Object {
	res := *seq
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func (seq *MappingSeq) GetType() *coretypes.Type {
	return TYPE.MappingSeq
}

func (seq *MappingSeq) Hash() uint32 {
	return hashOrdered(seq)
}

func (seq *MappingSeq) First() coretypes.Object {
	return seq.fn(seq.seq.First())
}

func (seq *MappingSeq) Rest() Seq {
	return &MappingSeq{
		seq: seq.seq.Rest(),
		fn:  seq.fn,
	}
}

func (seq *MappingSeq) IsEmpty() bool {
	return seq.seq.IsEmpty()
}

func (seq *MappingSeq) Cons(obj coretypes.Object) Seq {
	return &ConsSeq{first: obj, rest: seq}
}

func (seq *MappingSeq) SequentialMarker() {}

func (seq *LazySeq) Seq() Seq {
	return seq
}

func (seq *LazySeq) realize() {
	if seq.seq == nil {
		seq.seq = EnsureObjectIsSeqable(call0(seq.fn), "").Seq()
	}
}

func (seq *LazySeq) IsRealized() bool {
	return seq.seq != nil
}

func (seq *LazySeq) Equals(other interface{}) bool {
	return IsSeqEqual(seq, other)
}

func (seq *LazySeq) ToString(escape bool) string {
	return SeqToString(seq, escape)
}

func (seq *LazySeq) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *LazySeq) Format(w io.Writer, indent int) int {
	return formatSeq(seq, w, indent)
}

func (seq *LazySeq) WithMeta(meta Map) coretypes.Object {
	res := *seq
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func (seq *LazySeq) GetType() *coretypes.Type {
	return TYPE.LazySeq
}

func (seq *LazySeq) Hash() uint32 {
	return hashOrdered(seq)
}

func (seq *LazySeq) First() coretypes.Object {
	seq.realize()
	return seq.seq.First()
}

func (seq *LazySeq) Rest() Seq {
	seq.realize()
	return seq.seq.Rest()
}

func (seq *LazySeq) IsEmpty() bool {
	seq.realize()
	return seq.seq.IsEmpty()
}

func (seq *LazySeq) Cons(obj coretypes.Object) Seq {
	return &ConsSeq{first: obj, rest: seq}
}

func (seq *LazySeq) SequentialMarker() {}

func NewLazySeq(c coretypes.Callable) *LazySeq {
	return &LazySeq{fn: c}
}

func (seq *ArraySeq) Seq() Seq {
	return seq
}

func (seq *ArraySeq) Equals(other interface{}) bool {
	return IsSeqEqual(seq, other)
}

func (seq *ArraySeq) ToString(escape bool) string {
	return SeqToString(seq, escape)
}

func (seq *ArraySeq) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *ArraySeq) Format(w io.Writer, indent int) int {
	return formatSeq(seq, w, indent)
}

func (seq *ArraySeq) WithMeta(meta Map) coretypes.Object {
	res := *seq
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func (seq *ArraySeq) GetType() *coretypes.Type {
	return TYPE.ArraySeq
}

func (seq *ArraySeq) Hash() uint32 {
	return hashOrdered(seq)
}

func (seq *ArraySeq) First() coretypes.Object {
	if seq.IsEmpty() {
		return NIL
	}
	return seq.arr[seq.index]
}

func (seq *ArraySeq) Rest() Seq {
	if seq.index+1 < len(seq.arr) {
		return &ArraySeq{index: seq.index + 1, arr: seq.arr}
	}
	return EmptyList
}

func (seq *ArraySeq) IsEmpty() bool {
	return seq.index >= len(seq.arr)
}

func (seq *ArraySeq) Count() int {
	n := len(seq.arr) - seq.index
	if n < 0 {
		return 0
	}
	return n
}

func (seq *ArraySeq) Cons(obj coretypes.Object) Seq {
	return &ConsSeq{first: obj, rest: seq}
}

func (seq *ArraySeq) SequentialMarker() {}

func SeqToString(seq Seq, escape bool) string {
	b := bufferpool.Get()
	defer bufferpool.Put(b)
	b.WriteRune('(')
	for iter := iter(seq); iter.HasNext(); {
		b.WriteString(iter.Next().ToString(escape))
		if iter.HasNext() {
			b.WriteRune(' ')
		}
	}
	b.WriteRune(')')
	return b.String()
}

func (seq *ConsSeq) WithMeta(meta Map) coretypes.Object {
	res := *seq
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func (seq *ConsSeq) Seq() Seq {
	return seq
}

func (seq *ConsSeq) Equals(other interface{}) bool {
	return IsSeqEqual(seq, other)
}

func (seq *ConsSeq) ToString(escape bool) string {
	return SeqToString(seq, escape)
}

func (seq *ConsSeq) Pprint(w io.Writer, indent int) int {
	return pprintSeq(seq, w, indent)
}

func (seq *ConsSeq) Format(w io.Writer, indent int) int {
	return formatSeq(seq, w, indent)
}

func (seq *ConsSeq) GetType() *coretypes.Type {
	return TYPE.ConsSeq
}

func (seq *ConsSeq) Hash() uint32 {
	return hashOrdered(seq)
}

func (seq *ConsSeq) First() coretypes.Object {
	return seq.first
}

func (seq *ConsSeq) Rest() Seq {
	return seq.rest
}

func (seq *ConsSeq) IsEmpty() bool {
	return false
}

func (seq *ConsSeq) Cons(obj coretypes.Object) Seq {
	return &ConsSeq{first: obj, rest: seq}
}

func (seq *ConsSeq) SequentialMarker() {}

func NewConsSeq(first coretypes.Object, rest Seq) *ConsSeq {
	return &ConsSeq{
		first: first,
		rest:  rest,
	}
}

func iter(seq Seq) *SeqIterator {
	return &SeqIterator{seq: seq}
}

func (iter *SeqIterator) Next() coretypes.Object {
	res := iter.seq.First()
	iter.seq = iter.seq.Rest()
	return res
}

func (iter *SeqIterator) HasNext() bool {
	return !iter.seq.IsEmpty()
}

func Second(seq Seq) coretypes.Object {
	return seq.Rest().First()
}

func Third(seq Seq) coretypes.Object {
	return seq.Rest().Rest().First()
}

func Fourth(seq Seq) coretypes.Object {
	return seq.Rest().Rest().Rest().First()
}

func ToSlice(seq Seq) []coretypes.Object {
	res := make([]coretypes.Object, 0)
	for !seq.IsEmpty() {
		res = append(res, seq.First())
		seq = seq.Rest()
	}
	return res
}

func SeqCount(seq Seq) int {
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

func SeqNth(seq Seq, n int) coretypes.Object {
	if n < 0 {
		panic(RT.NewError(fmt.Sprintf("Negative index: %d", n)))
	}
	switch seq := seq.(type) {
	case *ArraySeq:
		idx := seq.index + n
		if idx >= 0 && idx < len(seq.arr) {
			return seq.arr[idx]
		}
		panic(RT.NewError(fmt.Sprintf("Index %d exceeds seq's length %d", n, len(seq.arr)-seq.index)))
	}
	i := n
	for !seq.IsEmpty() {
		if i == 0 {
			return seq.First()
		}
		seq = seq.Rest()
		i--
	}
	panic(RT.NewError(fmt.Sprintf("Index %d exceeds seq's length %d", n, (n - i))))
}

func SeqTryNth(seq Seq, n int, d coretypes.Object) coretypes.Object {
	if n < 0 {
		return d
	}
	switch seq := seq.(type) {
	case *ArraySeq:
		idx := seq.index + n
		if idx >= 0 && idx < len(seq.arr) {
			return seq.arr[idx]
		}
		return d
	}
	i := n
	for !seq.IsEmpty() {
		if i == 0 {
			return seq.First()
		}
		seq = seq.Rest()
		i--
	}
	return d
}

func hashUnordered(seq Seq, seed uint32) uint32 {
	for !seq.IsEmpty() {
		seed += seq.First().Hash()
		seq = seq.Rest()
	}
	h := coretypes.NewHash32()
	h.Write(hashutil.Uint32Bytes(seed))
	return h.Sum32()
}

func hashOrdered(seq Seq) uint32 {
	h := coretypes.NewHash32()
	for !seq.IsEmpty() {
		h.Write(hashutil.Uint32Bytes(seq.First().Hash()))
		seq = seq.Rest()
	}
	return h.Sum32()
}

func pprintSeq(seq Seq, w io.Writer, indent int) int {
	i := indent + 1
	fmt.Fprint(w, "(")
	for iter := iter(seq); iter.HasNext(); {
		i = pprintObject(iter.Next(), indent+1, w)
		if iter.HasNext() {
			fmt.Fprint(w, "\n")
			writeIndent(w, indent+1)
		}
	}
	fmt.Fprint(w, ")")
	return i + 1
}
