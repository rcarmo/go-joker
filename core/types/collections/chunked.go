package collections

import coretypes "github.com/rcarmo/go-joker/core/types"

func ChunkAppend[T any](arr []T, value T) ([]T, int) {
	arr = append(arr, value)
	return arr, len(arr)
}

func ChunkCount(off, end int) int { return end - off }

func ChunkNth[T any](arr []T, off, end, i int) (T, bool) {
	idx := off + i
	if idx < 0 || idx >= end {
		var zero T
		return zero, false
	}
	return arr[idx], true
}

func ChunkDropFirst[T any](arr []T, off, end int) (newOff int, ok bool) {
	if off+1 >= end {
		return off, false
	}
	return off + 1, true
}

func ChunkedConsNext(idx, chunkCount int, hasRest bool) (advanceChunk bool, nextIdx int, useRest bool) {
	if idx+1 < chunkCount {
		return false, idx + 1, false
	}
	if hasRest {
		return false, idx, true
	}
	return true, idx, false
}

func ChunkRestFromRest(rest coretypes.Seq, empty coretypes.Seq) coretypes.Seq {
	if rest != nil {
		return rest
	}
	return empty
}

func ChunkNextFromRest(rest coretypes.Seq, nilObj coretypes.Object) coretypes.Object {
	if rest != nil && !rest.IsEmpty() {
		return rest
	}
	return nilObj
}

func ChunkConsRest(restArg coretypes.Object, isNil func(coretypes.Object) bool) coretypes.Seq {
	if restArg != nil && !isNil(restArg) {
		if s, ok := restArg.(coretypes.Seqable); ok {
			return s.Seq()
		}
	}
	return nil
}

func ChunkFirstSingle(s coretypes.Seq) []coretypes.Object {
	if s.IsEmpty() {
		return nil
	}
	return []coretypes.Object{s.First()}
}

type ChunkBuffer struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	Arr    []coretypes.Object
	CountN int
}

func (cb *ChunkBuffer) ToString(bool) string                            { return "#object[ChunkBuffer]" }
func (cb *ChunkBuffer) Equals(other interface{}) bool                   { return cb == other }
func (cb *ChunkBuffer) GetType() *coretypes.Type                        { return coretypes.RuntimeTypes.ArrayVector }
func (cb *ChunkBuffer) Hash() uint32                                    { return 0 }
func (cb *ChunkBuffer) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return cb }
func (cb *ChunkBuffer) WithMeta(coretypes.Map) coretypes.Object         { return cb }

type ArrayChunk struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	Arr []coretypes.Object
	Off int
	End int
}

func (ac *ArrayChunk) ToString(bool) string                            { return "#object[ArrayChunk]" }
func (ac *ArrayChunk) Equals(other interface{}) bool                   { return ac == other }
func (ac *ArrayChunk) GetType() *coretypes.Type                        { return coretypes.RuntimeTypes.ArrayVector }
func (ac *ArrayChunk) Hash() uint32                                    { return 0 }
func (ac *ArrayChunk) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return ac }
func (ac *ArrayChunk) WithMeta(coretypes.Map) coretypes.Object         { return ac }

func (ac *ArrayChunk) Count() int { return ChunkCount(ac.Off, ac.End) }
func (ac *ArrayChunk) Nth(i int) coretypes.Object {
	if v, ok := ChunkNth(ac.Arr, ac.Off, ac.End, i); ok {
		return v
	}
	panic(coretypes.RuntimeError("ArrayChunk index out of bounds"))
}
func (ac *ArrayChunk) DropFirst() *ArrayChunk {
	if off, ok := ChunkDropFirst[coretypes.Object](ac.Arr, ac.Off, ac.End); ok {
		return &ArrayChunk{Arr: ac.Arr, Off: off, End: ac.End}
	}
	panic(coretypes.RuntimeError("dropFirst on empty chunk"))
}

type ChunkedCons struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	Chunk   *ArrayChunk
	RestSeq coretypes.Seq
	Idx     int
}

func (cc *ChunkedCons) ToString(escape bool) string {
	return SeqToString(cc, func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (cc *ChunkedCons) Equals(other interface{}) bool { return coretypes.IsSeqEqual(cc, other) }
func (cc *ChunkedCons) GetType() *coretypes.Type      { return coretypes.RuntimeTypes.LazySeq }
func (cc *ChunkedCons) Hash() uint32                  { return HashOrdered(cc) }
func (cc *ChunkedCons) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *cc
	res.Info = info
	return &res
}
func (cc *ChunkedCons) WithMeta(m coretypes.Map) coretypes.Object {
	res := *cc
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}
func (cc *ChunkedCons) Seq() coretypes.Seq      { return cc }
func (cc *ChunkedCons) SequentialMarker()       {}
func (cc *ChunkedCons) First() coretypes.Object { return cc.Chunk.Nth(cc.Idx) }
func (cc *ChunkedCons) Rest() coretypes.Seq {
	_, nextIdx, useRest := ChunkedConsNext(cc.Idx, cc.Chunk.Count(), cc.RestSeq != nil)
	if useRest {
		return cc.RestSeq
	}
	if nextIdx != cc.Idx {
		return &ChunkedCons{Chunk: cc.Chunk, RestSeq: cc.RestSeq, Idx: nextIdx}
	}
	return EmptyList
}
func (cc *ChunkedCons) IsEmpty() bool { return false }
func (cc *ChunkedCons) Cons(obj coretypes.Object) coretypes.Seq {
	return &ConsSeq{FirstValue: obj, RestValue: cc}
}
