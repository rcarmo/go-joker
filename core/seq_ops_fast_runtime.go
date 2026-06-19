package core

import (
	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

// ---- seq_ops_fast.go ----
// seq_ops_fast.go — Fast map/filter/take seq wrappers for reducible pipelines.

type FilteringSeq struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	seq  coretypes.Seq
	pred coretypes.Callable
}

type TakeSeq struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	seq coretypes.Seq
	n   int
}

func (s *FilteringSeq) ToString(escape bool) string {
	return corecollections.SeqToString(s, func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (s *FilteringSeq) Equals(other interface{}) bool { return coretypes.IsSeqEqual(s, other) }
func (s *FilteringSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *s
	res.Info = info
	return &res
}
func (s *FilteringSeq) GetType() *coretypes.Type { return TYPE.LazySeq }
func (s *FilteringSeq) Hash() uint32             { return corecollections.HashOrdered(s) }
func (s *FilteringSeq) WithMeta(m coretypes.Map) coretypes.Object {
	res := *s
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}
func (s *FilteringSeq) Seq() coretypes.Seq      { return s }
func (s *FilteringSeq) SequentialMarker()       {}
func (s *FilteringSeq) IsEmpty() bool           { return s.nextSeq().IsEmpty() }
func (s *FilteringSeq) First() coretypes.Object { return s.nextSeq().First() }
func (s *FilteringSeq) Rest() coretypes.Seq {
	ns := s.nextSeq()
	if ns.IsEmpty() {
		return corecollections.EmptyList
	}
	return &FilteringSeq{seq: ns.Rest(), pred: s.pred}
}
func (s *FilteringSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &corecollections.ConsSeq{FirstValue: obj, RestValue: s}
}

func (s *FilteringSeq) nextSeq() coretypes.Seq {
	cur := s.seq
	for !cur.IsEmpty() {
		if corert.ToBool(call1(s.pred, cur.First())) {
			return cur
		}
		cur = cur.Rest()
	}
	return corecollections.EmptyList
}

func (s *FilteringSeq) Reduce(f coretypes.Callable) coretypes.Object {
	cur := s.seq
	for !cur.IsEmpty() {
		v := cur.First()
		if corert.ToBool(call1(s.pred, v)) {
			acc := v
			cur = cur.Rest()
			for !cur.IsEmpty() {
				v = cur.First()
				if corert.ToBool(call1(s.pred, v)) {
					acc = call2(f, acc, v)
					if corert.IsReduced(acc) {
						return corert.DerefReduced(acc)
					}
				}
				cur = cur.Rest()
			}
			return acc
		}
		cur = cur.Rest()
	}
	return call0(f)
}

func (s *FilteringSeq) ReduceInit(f coretypes.Callable, init coretypes.Object) coretypes.Object {
	acc := init
	cur := s.seq
	for !cur.IsEmpty() {
		v := cur.First()
		if corert.ToBool(call1(s.pred, v)) {
			acc = call2(f, acc, v)
			if corert.IsReduced(acc) {
				return corert.DerefReduced(acc)
			}
		}
		cur = cur.Rest()
	}
	return acc
}

func (s *TakeSeq) ToString(escape bool) string {
	return corecollections.SeqToString(s, func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (s *TakeSeq) Equals(other interface{}) bool { return coretypes.IsSeqEqual(s, other) }
func (s *TakeSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *s
	res.Info = info
	return &res
}
func (s *TakeSeq) GetType() *coretypes.Type { return TYPE.LazySeq }
func (s *TakeSeq) Hash() uint32             { return corecollections.HashOrdered(s) }
func (s *TakeSeq) WithMeta(m coretypes.Map) coretypes.Object {
	res := *s
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}
func (s *TakeSeq) Seq() coretypes.Seq      { return s }
func (s *TakeSeq) SequentialMarker()       {}
func (s *TakeSeq) IsEmpty() bool           { return s.n <= 0 || s.seq.IsEmpty() }
func (s *TakeSeq) First() coretypes.Object { return s.seq.First() }
func (s *TakeSeq) Rest() coretypes.Seq {
	if s.n <= 1 {
		return corecollections.EmptyList
	}
	return &TakeSeq{seq: s.seq.Rest(), n: s.n - 1}
}
func (s *TakeSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &corecollections.ConsSeq{FirstValue: obj, RestValue: s}
}

func (s *TakeSeq) Reduce(f coretypes.Callable) coretypes.Object {
	if result, ok := s.reduceFused(f); ok {
		return result
	}
	if s.IsEmpty() {
		return call0(f)
	}
	acc := s.seq.First()
	cur := s.seq.Rest()
	for i := 1; i < s.n && !cur.IsEmpty(); i++ {
		acc = call2(f, acc, cur.First())
		if corert.IsReduced(acc) {
			return corert.DerefReduced(acc)
		}
		cur = cur.Rest()
	}
	return acc
}

func (s *TakeSeq) reduceFused(f coretypes.Callable) (coretypes.Object, bool) {
	return nil, false
}

func (s *TakeSeq) ReduceInit(f coretypes.Callable, init coretypes.Object) coretypes.Object {
	acc := init
	cur := s.seq
	for i := 0; i < s.n && !cur.IsEmpty(); i++ {
		acc = call2(f, acc, cur.First())
		if corert.IsReduced(acc) {
			return corert.DerefReduced(acc)
		}
		cur = cur.Rest()
	}
	return acc
}

func chunkedMapSeq(f coretypes.Callable, src coretypes.Seq) coretypes.Seq {
	if src == nil || src.IsEmpty() {
		return corecollections.EmptyList
	}
	buf := make([]coretypes.Object, 0, 32)
	cur := src
	for len(buf) < 32 && !cur.IsEmpty() {
		buf = append(buf, call1(f, cur.First()))
		cur = cur.Rest()
	}
	chunk := &corecollections.ArrayChunk{Arr: buf, Off: 0, End: len(buf)}
	var rest coretypes.Seq
	if !cur.IsEmpty() {
		restCur := cur
		rest = &corecollections.LazySeq{Fn: Proc{Name: "procChunkedMapRest", Fn: func(args []coretypes.Object) coretypes.Object {
			return chunkedMapSeq(f, restCur)
		}}}
	}
	return &corecollections.ChunkedCons{Chunk: chunk, RestSeq: rest, Idx: 0}
}

func chunkedFilterSeq(pred coretypes.Callable, src coretypes.Seq) coretypes.Seq {
	cur := src
	for {
		if cur == nil || cur.IsEmpty() {
			return corecollections.EmptyList
		}
		buf := make([]coretypes.Object, 0, 32)
		for len(buf) < 32 && !cur.IsEmpty() {
			v := cur.First()
			if corert.ToBool(call1(pred, v)) {
				buf = append(buf, v)
			}
			cur = cur.Rest()
		}
		if len(buf) > 0 {
			chunk := &corecollections.ArrayChunk{Arr: buf, Off: 0, End: len(buf)}
			var rest coretypes.Seq
			if !cur.IsEmpty() {
				restCur := cur
				rest = &corecollections.LazySeq{Fn: Proc{Name: "procChunkedFilterRest", Fn: func(args []coretypes.Object) coretypes.Object {
					return chunkedFilterSeq(pred, restCur)
				}}}
			}
			return &corecollections.ChunkedCons{Chunk: chunk, RestSeq: rest, Idx: 0}
		}
	}
}

func maybeOverrideSeqOps() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}
	mapVr, filterVr, takeVr := ns.Resolve("map"), ns.Resolve("filter"), ns.Resolve("take")
	if mapVr == nil || filterVr == nil || takeVr == nil {
		return
	}
	mapOrig, mapOK := mapVr.Value.(coretypes.Callable)
	filterOrig, filterOK := filterVr.Value.(coretypes.Callable)
	takeOrig, takeOK := takeVr.Value.(coretypes.Callable)
	if !mapOK || !filterOK || !takeOK {
		return
	}
	if p, ok := mapVr.Value.(Proc); ok && p.Name == "procMapSeqFast" {
		return
	}

	mapVr.Value = Proc{Name: "procMapSeqFast", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			return makeMapTransducer(coretypes.EnsureArgIsCallable(args, 0))
		}
		if len(args) == 2 {
			f := coretypes.EnsureArgIsCallable(args, 0)
			s := coretypes.EnsureObjectIsSeqable(args[1], "map requires seqable").Seq()
			if _, ok := s.(*corecollections.ChunkedCons); ok {
				return chunkedMapSeq(f, s)
			}
			return &corecollections.MappingSeq{SeqValue: s, Fn: func(o coretypes.Object) coretypes.Object { return call1(f, o) }}
		}
		return mapOrig.Call(args)
	}}
	filterVr.Value = Proc{Name: "procFilterSeqFast", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			return makeFilterTransducer(coretypes.EnsureArgIsCallable(args, 0))
		}
		if len(args) == 2 {
			pred := coretypes.EnsureArgIsCallable(args, 0)
			s := coretypes.EnsureArgIsSeqable(args, 1).Seq()
			if _, ok := s.(*corecollections.ChunkedCons); ok {
				return chunkedFilterSeq(pred, s)
			}
			return &FilteringSeq{seq: s, pred: pred}
		}
		return filterOrig.Call(args)
	}}
	takeVr.Value = Proc{Name: "procTakeSeqFast", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			return makeTakeTransducer(coretypes.EnsureObjectIsNumber(args[0], "").Int().I)
		}
		if len(args) == 2 {
			return &TakeSeq{seq: coretypes.EnsureObjectIsSeqable(args[1], "take requires seqable").Seq(), n: coretypes.EnsureObjectIsNumber(args[0], "").Int().I}
		}
		return takeOrig.Call(args)
	}}
}
