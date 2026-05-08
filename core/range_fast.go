package core

import "sync"

// range_fast.go — Efficient Range type that implements Reduce for fast numeric reduce.

var hotReducerFnCache sync.Map // *Fn -> reducer proc name string

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
	return r.chunkedSeqFrom(r.start)
}

func (r *IntRange) chunkedSeqFrom(cur int) Seq {
	if !r.contains(cur) {
		return EmptyList
	}
	buf := make([]Object, 0, 32)
	v := cur
	for len(buf) < 32 && r.contains(v) {
		buf = append(buf, Int{I: v})
		v += r.step
	}
	chunk := &ArrayChunk{arr: buf, off: 0, end: len(buf)}
	var rest Seq
	if r.contains(v) {
		rest = &LazySeq{fn: Proc{Name: "procIntRangeChunkRest", Fn: func(args []Object) Object {
			return r.chunkedSeqFrom(v)
		}}}
	}
	return &ChunkedCons{chunk: chunk, rest: rest, idx: 0}
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
		acc = call2(f, acc, Int{I: i})
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
		acc = call2(f, acc, Int{I: i})
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
	if result, ok := r.reduceMapAssocFast(f, init); ok {
		return result, true
	}
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

func (r *IntRange) reduceMapAssocFast(f Callable, init Object) (Object, bool) {
	fn, ok := f.(*Fn)
	if !ok || fn == nil || fn.fnExpr == nil || len(fn.fnExpr.arities) != 1 || fn.fnExpr.variadic != nil {
		return nil, false
	}
	m, ok := init.(Map)
	if !ok {
		return nil, false
	}
	arity := fn.fnExpr.arities[0]
	if len(arity.args) != 2 || len(arity.body) != 1 {
		return nil, false
	}
	pf := guessFnParamFrame(arity.body, 2)
	if pf < 0 {
		pf = 1
	}
	call, ok := arity.body[0].(*CallExpr)
	if !ok || len(call.args) != 3 {
		return nil, false
	}
	vref, ok := call.callable.(*VarRefExpr)
	if !ok || coreVarToProcName(vref.vr) != "procAssoc" {
		return nil, false
	}
	base, ok := call.args[0].(*BindingExpr)
	if !ok || base.binding.frame != pf || base.binding.index != 0 {
		return nil, false
	}
	keyFn := compileIntExpr2(call.args[1], nil, pf, &nativeRecursiveEntry{arity: 2})
	valFn := compileIntExpr2(call.args[2], nil, pf, &nativeRecursiveEntry{arity: 2})
	if keyFn == nil || valFn == nil {
		return nil, false
	}
	tm := MapToTransient(m)
	for i := r.start; r.contains(i); i += r.step {
		tm.AssocInPlace(Int{I: keyFn(0, i)}, Int{I: valFn(0, i)})
	}
	return tm.ToPersistent(), true
}

func hotReducerName(f Callable) string {
	switch c := f.(type) {
	case Proc:
		return c.Name
	case *Fn:
		if c.defVar != nil {
			if proc := hotReducerSymbol(c.defVar.name.ToString(false)); proc != "" {
				return proc
			}
		}
		if cached, ok := hotReducerFnCache.Load(c); ok {
			return cached.(string)
		}
		if proc := bindHotReducerDefVar(c); proc != "" {
			hotReducerFnCache.Store(c, proc)
			return proc
		}
	case *Var:
		return hotReducerSymbol(c.name.ToString(false))
	}
	return ""
}

func findFnVarName(fn *Fn) string {
	if fn != nil && fn.defVar != nil {
		return fn.defVar.name.ToString(false)
	}
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

func bindHotReducerDefVar(fn *Fn) string {
	if fn == nil {
		return ""
	}
	for _, ns := range []*Namespace{GLOBAL_ENV.CurrentNamespace(), GLOBAL_ENV.CoreNamespace} {
		if ns == nil {
			continue
		}
		for _, vr := range ns.Mappings() {
			if vr.Value == fn {
				proc := hotReducerSymbol(vr.name.ToString(false))
				if proc != "" {
					fn.defVar = vr
					return proc
				}
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
	case "<":
		return "procLt"
	case "<=":
		return "procLte"
	case ">":
		return "procGt"
	case ">=":
		return "procGte"
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
