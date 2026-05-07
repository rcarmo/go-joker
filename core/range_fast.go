package core

// range_fast.go — Efficient Range type that implements Reduce for fast numeric reduce.
// This eliminates lazy seq overhead for (reduce f init (range n)) patterns.

// IntRange represents a range of integers [start, end) with step.
type IntRange struct {
	InfoHolder
	MetaHolder
	start, end, step int
}

func NewIntRange(start, end, step int) *IntRange {
	return &IntRange{start: start, end: end, step: step}
}

func (r *IntRange) ToString(escape bool) string {
	return SeqToString(r.Seq(), escape)
}

func (r *IntRange) Equals(other interface{}) bool {
	return IsSeqEqual(r.Seq(), other)
}

func (r *IntRange) GetInfo() *ObjectInfo { return r.info }
func (r *IntRange) WithInfo(i *ObjectInfo) Object {
	res := *r
	res.info = i
	return &res
}
func (r *IntRange) GetType() *Type { return TYPE.LazySeq } // compatible type
func (r *IntRange) Hash() uint32   { return hashOrdered(r.Seq()) }

func (r *IntRange) WithMeta(m Map) Object {
	res := *r
	res.meta = SafeMerge(res.meta, m)
	return &res
}

func (r *IntRange) Seq() Seq {
	if r.step > 0 && r.start >= r.end {
		return EmptyList
	}
	if r.step < 0 && r.start <= r.end {
		return EmptyList
	}
	return &intRangeSeq{r: r, cur: r.start}
}

func (r *IntRange) Count() int {
	if r.step > 0 {
		n := (r.end - r.start + r.step - 1) / r.step
		if n < 0 {
			return 0
		}
		return n
	}
	if r.step < 0 {
		n := (r.start - r.end - r.step - 1) / (-r.step)
		if n < 0 {
			return 0
		}
		return n
	}
	return 0 // step==0 is infinite, but we return 0 for safety
}

// Reduce interface — this is the key performance win
func (r *IntRange) reduce(f Callable) Object {
	if r.step > 0 && r.start >= r.end {
		return f.Call(nil)
	}
	if r.step < 0 && r.start <= r.end {
		return f.Call(nil)
	}
	acc := Object(Int{I: r.start})
	for i := r.start + r.step; (r.step > 0 && i < r.end) || (r.step < 0 && i > r.end); i += r.step {
		acc = f.Call([]Object{acc, Int{I: i}})
		if isReducedObj(acc) {
			return unreducedObj(acc)
		}
	}
	return acc
}

func (r *IntRange) reduceInit(f Callable, init Object) Object {
	// Specialized fast path for core/+ with integer init
	switch p := f.(type) {
	case Proc:
		if p.Name == "procAdd" {
			if initInt, ok := init.(Int); ok {
				acc := initInt.I
				for i := r.start; (r.step > 0 && i < r.end) || (r.step < 0 && i > r.end); i += r.step {
					acc += i
				}
				return Int{I: acc}
			}
		}
	case *Fn:
		// Check if this is the core/+ var (single-arity fn that delegates to procAdd)
		if fn := f.(*Fn); fn.fnExpr != nil && len(fn.fnExpr.arities) > 0 {
			// For core math fns, try detecting by checking var name
			// Fall through to generic path
		}
	}
	acc := init
	for i := r.start; (r.step > 0 && i < r.end) || (r.step < 0 && i > r.end); i += r.step {
		acc = f.Call([]Object{acc, Int{I: i}})
		if isReducedObj(acc) {
			return unreducedObj(acc)
		}
	}
	return acc
}

func (r *IntRange) sequential() {}

// intRangeSeq is the lazy seq view of an IntRange
type intRangeSeq struct {
	InfoHolder
	MetaHolder
	r   *IntRange
	cur int
}

func (s *intRangeSeq) ToString(escape bool) string   { return SeqToString(s, escape) }
func (s *intRangeSeq) Equals(other interface{}) bool { return IsSeqEqual(s, other) }
func (s *intRangeSeq) WithInfo(info *ObjectInfo) Object {
	res := *s
	res.info = info
	return &res
}
func (s *intRangeSeq) GetType() *Type { return TYPE.LazySeq }
func (s *intRangeSeq) Hash() uint32   { return hashOrdered(s) }
func (s *intRangeSeq) WithMeta(m Map) Object {
	res := *s
	res.meta = SafeMerge(res.meta, m)
	return &res
}
func (s *intRangeSeq) Seq() Seq { return s }

func (s *intRangeSeq) First() Object {
	return Int{I: s.cur}
}

func (s *intRangeSeq) Rest() Seq {
	next := s.cur + s.r.step
	if s.r.step > 0 && next >= s.r.end {
		return EmptyList
	}
	if s.r.step < 0 && next <= s.r.end {
		return EmptyList
	}
	return &intRangeSeq{r: s.r, cur: next}
}

func (s *intRangeSeq) IsEmpty() bool {
	if s.r.step > 0 {
		return s.cur >= s.r.end
	}
	return s.cur <= s.r.end
}

func (s *intRangeSeq) Cons(obj Object) Seq {
	return &ConsSeq{first: obj, rest: s}
}

func (s *intRangeSeq) sequential() {}

// Override the core/range function to return IntRange for integer arguments
func init() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	rangeVr := ns.Resolve("range")
	if rangeVr == nil {
		return
	}

	origRange, origOK := rangeVr.Value.(Callable)
	if !origOK {
		return
	}

	rangeVr.Value = Proc{Name: "procRangeFast", Fn: func(args []Object) Object {
		switch len(args) {
		case 0:
			// (range) — infinite range, delegate to original
			return origRange.Call(args)
		case 1:
			// (range end)
			if end, ok := args[0].(Int); ok {
				return NewIntRange(0, end.I, 1)
			}
			return origRange.Call(args)
		case 2:
			// (range start end)
			if start, ok := args[0].(Int); ok {
				if end, ok := args[1].(Int); ok {
					return NewIntRange(start.I, end.I, 1)
				}
			}
			return origRange.Call(args)
		case 3:
			// (range start end step)
			if start, ok := args[0].(Int); ok {
				if end, ok := args[1].(Int); ok {
					if step, ok := args[2].(Int); ok {
						return NewIntRange(start.I, end.I, step.I)
					}
				}
			}
			return origRange.Call(args)
		default:
			return origRange.Call(args)
		}
	}}
}
