package core

// range_fast.go — Efficient Range type that implements Reduce for fast numeric reduce.

// IntRange represents a range of integers [start, end) with step.
type IntRange struct {
	InfoHolder
	MetaHolder
	start, end, step int
}

func NewIntRange(start, end, step int) *IntRange {
	return &IntRange{start: start, end: end, step: step}
}

func (r *IntRange) ToString(escape bool) string   { return SeqToString(r.Seq(), escape) }
func (r *IntRange) Equals(other interface{}) bool { return IsSeqEqual(r.Seq(), other) }
func (r *IntRange) WithInfo(i *ObjectInfo) Object { res := *r; res.info = i; return &res }
func (r *IntRange) GetType() *Type                { return TYPE.LazySeq }
func (r *IntRange) Hash() uint32                  { return hashOrdered(r.Seq()) }
func (r *IntRange) WithMeta(m Map) Object         { res := *r; res.meta = SafeMerge(res.meta, m); return &res }
func (r *IntRange) sequential()                   {}

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
	if r.step == 0 {
		panic(RT.NewError("range: step must not be 0"))
	}
	return 0
}

func (r *IntRange) reduce(f Callable) Object {
	if r.isEmpty() {
		return f.Call(nil)
	}
	if result, ok := r.reduceFast(f); ok {
		return result
	}
	acc := Object(Int{I: r.start})
	for i := r.start + r.step; r.contains(i); i += r.step {
		acc = f.Call([]Object{acc, Int{I: i}})
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
	}
	return acc
}

func (r *IntRange) reduceInit(f Callable, init Object) Object {
	if result, ok := r.reduceInitFast(f, init); ok {
		return result
	}
	acc := init
	for i := r.start; r.contains(i); i += r.step {
		acc = f.Call([]Object{acc, Int{I: i}})
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
	}
	return acc
}

func (r *IntRange) isEmpty() bool {
	return (r.step > 0 && r.start >= r.end) || (r.step < 0 && r.start <= r.end)
}

func (r *IntRange) contains(i int) bool {
	return (r.step > 0 && i < r.end) || (r.step < 0 && i > r.end)
}

func (r *IntRange) reduceFast(f Callable) (Object, bool) {
	name := hotReducerName(f)
	switch name {
	case "procAdd", "procunchecked-add", "procunchecked-add-int":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			acc += i
		}
		return Int{I: acc}, true
	case "procMultiply", "procunchecked-multiply", "procunchecked-multiply-int":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			acc *= i
		}
		return Int{I: acc}, true
	case "procMax":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			if i > acc {
				acc = i
			}
		}
		return Int{I: acc}, true
	case "procMin":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			if i < acc {
				acc = i
			}
		}
		return Int{I: acc}, true
	}
	return nil, false
}

func (r *IntRange) reduceInitFast(f Callable, init Object) (Object, bool) {
	name := hotReducerName(f)
	switch acc := init.(type) {
	case Int:
		v := acc.I
		switch name {
		case "procAdd", "procunchecked-add", "procunchecked-add-int":
			for i := r.start; r.contains(i); i += r.step {
				v += i
			}
			return Int{I: v}, true
		case "procMultiply", "procunchecked-multiply", "procunchecked-multiply-int":
			for i := r.start; r.contains(i); i += r.step {
				v *= i
			}
			return Int{I: v}, true
		case "procMax":
			for i := r.start; r.contains(i); i += r.step {
				if i > v {
					v = i
				}
			}
			return Int{I: v}, true
		case "procMin":
			for i := r.start; r.contains(i); i += r.step {
				if i < v {
					v = i
				}
			}
			return Int{I: v}, true
		}
	case Double:
		v := acc.D
		switch name {
		case "procAdd":
			for i := r.start; r.contains(i); i += r.step {
				v += float64(i)
			}
			return Double{D: v}, true
		case "procMultiply":
			for i := r.start; r.contains(i); i += r.step {
				v *= float64(i)
			}
			return Double{D: v}, true
		case "procMax":
			for i := r.start; r.contains(i); i += r.step {
				fi := float64(i)
				if fi > v {
					v = fi
				}
			}
			return Double{D: v}, true
		case "procMin":
			for i := r.start; r.contains(i); i += r.step {
				fi := float64(i)
				if fi < v {
					v = fi
				}
			}
			return Double{D: v}, true
		}
	}
	return nil, false
}

func hotReducerName(f Callable) string {
	switch c := f.(type) {
	case Proc:
		return c.Name
	case *Fn:
		if c.defVar != nil {
			return hotReducerSymbol(c.defVar.name.ToString(false))
		}
		if name := findFnVarName(c); name != "" {
			return hotReducerSymbol(name)
		}
	case *Var:
		return hotReducerSymbol(c.name.ToString(false))
	}
	return ""
}

func findFnVarName(fn *Fn) string {
	if fn == nil {
		return ""
	}
	if ns := GLOBAL_ENV.CurrentNamespace(); ns != nil {
		for _, vr := range ns.Mappings() {
			if vr.Value == fn {
				return vr.name.ToString(false)
			}
		}
	}
	if ns := GLOBAL_ENV.CoreNamespace; ns != nil {
		for _, vr := range ns.Mappings() {
			if vr.Value == fn {
				return vr.name.ToString(false)
			}
		}
	}
	return ""
}

func hotReducerSymbol(sym string) string {
	switch sym {
	case "+":
		return "procAdd"
	case "*":
		return "procMultiply"
	case "max":
		return "procMax"
	case "min":
		return "procMin"
	case "unchecked-add", "unchecked-add-int":
		return "procunchecked-add"
	case "unchecked-multiply", "unchecked-multiply-int":
		return "procunchecked-multiply"
	}
	return ""
}

// intRangeSeq is the lazy seq view of an IntRange
type intRangeSeq struct {
	InfoHolder
	MetaHolder
	r   *IntRange
	cur int
}

func (s *intRangeSeq) ToString(escape bool) string      { return SeqToString(s, escape) }
func (s *intRangeSeq) Equals(other interface{}) bool    { return IsSeqEqual(s, other) }
func (s *intRangeSeq) WithInfo(info *ObjectInfo) Object { res := *s; res.info = info; return &res }
func (s *intRangeSeq) GetType() *Type                   { return TYPE.LazySeq }
func (s *intRangeSeq) Hash() uint32                     { return hashOrdered(s) }
func (s *intRangeSeq) WithMeta(m Map) Object {
	res := *s
	res.meta = SafeMerge(res.meta, m)
	return &res
}
func (s *intRangeSeq) Seq() Seq      { return s }
func (s *intRangeSeq) sequential()   {}
func (s *intRangeSeq) First() Object { return Int{I: s.cur} }
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
	if s.r.step == 0 {
		return false // infinite range
	}
	if s.r.step > 0 {
		return s.cur >= s.r.end
	}
	return s.cur <= s.r.end
}
func (s *intRangeSeq) Cons(obj Object) Seq { return &ConsSeq{first: obj, rest: s} }

// maybeOverrideRange installs the IntRange-backed range wrapper after core.joke is loaded.
// It may be called multiple times; it only wraps the original range once.
func maybeOverrideRange() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}
	rangeVr := ns.Resolve("range")
	if rangeVr == nil {
		return
	}
	if p, ok := rangeVr.Value.(Proc); ok && p.Name == "procRangeFast" {
		return
	}
	origRange, ok := rangeVr.Value.(Callable)
	if !ok {
		return
	}

	rangeVr.Value = Proc{Name: "procRangeFast", Fn: func(args []Object) Object {
		switch len(args) {
		case 1:
			if end, ok := args[0].(Int); ok {
				return NewIntRange(0, end.I, 1)
			}
		case 2:
			if start, ok := args[0].(Int); ok {
				if end, ok := args[1].(Int); ok {
					return NewIntRange(start.I, end.I, 1)
				}
			}
		case 3:
			if start, ok := args[0].(Int); ok {
				if end, ok := args[1].(Int); ok {
					if step, ok := args[2].(Int); ok && step.I != 0 {
						return NewIntRange(start.I, end.I, step.I)
					}
				}
			}
		}
		return origRange.Call(args)
	}}
}
