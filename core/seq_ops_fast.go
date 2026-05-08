package core

// seq_ops_fast.go — Fast map/filter/take seq wrappers for reducible pipelines.

type FilteringSeq struct {
	InfoHolder
	MetaHolder
	seq  Seq
	pred Callable
}

type TakeSeq struct {
	InfoHolder
	MetaHolder
	seq Seq
	n   int
}

func (s *FilteringSeq) ToString(escape bool) string      { return SeqToString(s, escape) }
func (s *FilteringSeq) Equals(other interface{}) bool    { return IsSeqEqual(s, other) }
func (s *FilteringSeq) WithInfo(info *ObjectInfo) Object { res := *s; res.info = info; return &res }
func (s *FilteringSeq) GetType() *Type                   { return TYPE.LazySeq }
func (s *FilteringSeq) Hash() uint32                     { return hashOrdered(s) }
func (s *FilteringSeq) WithMeta(m Map) Object {
	res := *s
	res.meta = SafeMerge(res.meta, m)
	return &res
}
func (s *FilteringSeq) Seq() Seq      { return s }
func (s *FilteringSeq) sequential()   {}
func (s *FilteringSeq) IsEmpty() bool { return s.nextSeq().IsEmpty() }
func (s *FilteringSeq) First() Object { return s.nextSeq().First() }
func (s *FilteringSeq) Rest() Seq {
	ns := s.nextSeq()
	if ns.IsEmpty() {
		return EmptyList
	}
	return &FilteringSeq{seq: ns.Rest(), pred: s.pred}
}
func (s *FilteringSeq) Cons(obj Object) Seq { return &ConsSeq{first: obj, rest: s} }

func (s *FilteringSeq) nextSeq() Seq {
	cur := s.seq
	for !cur.IsEmpty() {
		if ToBool(call1(s.pred, cur.First())) {
			return cur
		}
		cur = cur.Rest()
	}
	return EmptyList
}

func (s *FilteringSeq) reduce(f Callable) Object {
	cur := s.seq
	for !cur.IsEmpty() {
		v := cur.First()
		if ToBool(call1(s.pred, v)) {
			acc := v
			cur = cur.Rest()
			for !cur.IsEmpty() {
				v = cur.First()
				if ToBool(call1(s.pred, v)) {
					acc = call2(f, acc, v)
					if IsReduced(acc) {
						return DerefReduced(acc)
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

func (s *FilteringSeq) reduceInit(f Callable, init Object) Object {
	acc := init
	cur := s.seq
	for !cur.IsEmpty() {
		v := cur.First()
		if ToBool(call1(s.pred, v)) {
			acc = call2(f, acc, v)
			if IsReduced(acc) {
				return DerefReduced(acc)
			}
		}
		cur = cur.Rest()
	}
	return acc
}

func (s *TakeSeq) ToString(escape bool) string      { return SeqToString(s, escape) }
func (s *TakeSeq) Equals(other interface{}) bool    { return IsSeqEqual(s, other) }
func (s *TakeSeq) WithInfo(info *ObjectInfo) Object { res := *s; res.info = info; return &res }
func (s *TakeSeq) GetType() *Type                   { return TYPE.LazySeq }
func (s *TakeSeq) Hash() uint32                     { return hashOrdered(s) }
func (s *TakeSeq) WithMeta(m Map) Object            { res := *s; res.meta = SafeMerge(res.meta, m); return &res }
func (s *TakeSeq) Seq() Seq                         { return s }
func (s *TakeSeq) sequential()                      {}
func (s *TakeSeq) IsEmpty() bool                    { return s.n <= 0 || s.seq.IsEmpty() }
func (s *TakeSeq) First() Object                    { return s.seq.First() }
func (s *TakeSeq) Rest() Seq {
	if s.n <= 1 {
		return EmptyList
	}
	return &TakeSeq{seq: s.seq.Rest(), n: s.n - 1}
}
func (s *TakeSeq) Cons(obj Object) Seq { return &ConsSeq{first: obj, rest: s} }

func (s *TakeSeq) reduce(f Callable) Object {
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
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
		cur = cur.Rest()
	}
	return acc
}

func (s *TakeSeq) reduceFused(f Callable) (Object, bool) {
	return nil, false
}

func (s *TakeSeq) reduceInit(f Callable, init Object) Object {
	acc := init
	cur := s.seq
	for i := 0; i < s.n && !cur.IsEmpty(); i++ {
		acc = call2(f, acc, cur.First())
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
		cur = cur.Rest()
	}
	return acc
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
	mapOrig, mapOK := mapVr.Value.(Callable)
	filterOrig, filterOK := filterVr.Value.(Callable)
	takeOrig, takeOK := takeVr.Value.(Callable)
	if !mapOK || !filterOK || !takeOK {
		return
	}
	if p, ok := mapVr.Value.(Proc); ok && p.Name == "procMapSeqFast" {
		return
	}

	mapVr.Value = Proc{Name: "procMapSeqFast", Fn: func(args []Object) Object {
		if len(args) == 1 {
			return makeMapTransducer(EnsureArgIsCallable(args, 0))
		}
		if len(args) == 2 {
			f := EnsureArgIsCallable(args, 0)
			s := EnsureObjectIsSeqable(args[1], "map requires seqable").Seq()
			return &MappingSeq{seq: s, fn: func(o Object) Object { return call1(f, o) }}
		}
		return mapOrig.Call(args)
	}}
	filterVr.Value = Proc{Name: "procFilterSeqFast", Fn: func(args []Object) Object {
		if len(args) == 1 {
			return makeFilterTransducer(EnsureArgIsCallable(args, 0))
		}
		if len(args) == 2 {
			return &FilteringSeq{seq: EnsureArgIsSeqable(args, 1).Seq(), pred: EnsureArgIsCallable(args, 0)}
		}
		return filterOrig.Call(args)
	}}
	takeVr.Value = Proc{Name: "procTakeSeqFast", Fn: func(args []Object) Object {
		if len(args) == 1 {
			return makeTakeTransducer(EnsureObjectIsNumber(args[0], "").Int().I)
		}
		if len(args) == 2 {
			return &TakeSeq{seq: EnsureObjectIsSeqable(args[1], "take requires seqable").Seq(), n: EnsureObjectIsNumber(args[0], "").Int().I}
		}
		return takeOrig.Call(args)
	}}
}
