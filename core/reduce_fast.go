package core

import (
	corestr "github.com/rcarmo/go-joker/core/string"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"sync"
)

// ---- reduce_fast.go ----
// reduce_fast.go — Seq-walking reduce fallback + IntRange creation at reduce time.

func seqReduceInit(s Seq, f coretypes.Callable, init Object) Object {
	acc := init
	for !s.IsEmpty() {
		acc = call2(f, acc, s.First())
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
		s = s.Rest()
	}
	return acc
}

func seqReduce(s Seq, f coretypes.Callable) Object {
	if s.IsEmpty() {
		return f.Call(nil)
	}
	acc := s.First()
	s = s.Rest()
	for !s.IsEmpty() {
		acc = call2(f, acc, s.First())
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
		s = s.Rest()
	}
	return acc
}

// LazySeq coretypes.Reduce support — implements the coretypes.Reduce interface so (reduce f init lazy-seq) works.
func (seq *LazySeq) Reduce(f coretypes.Callable) Object {
	return seqReduce(seq.Seq(), f)
}

func (seq *LazySeq) ReduceInit(f coretypes.Callable, init Object) Object {
	return seqReduceInit(seq.Seq(), f, init)
}

// ConsSeq coretypes.Reduce support
func (seq *ConsSeq) Reduce(f coretypes.Callable) Object {
	return seqReduce(seq, f)
}

func (seq *ConsSeq) ReduceInit(f coretypes.Callable, init Object) Object {
	return seqReduceInit(seq, f, init)
}

// MappingSeq coretypes.Reduce support
func (seq *MappingSeq) Reduce(f coretypes.Callable) Object {
	if seq.seq.IsEmpty() {
		return call0(f)
	}
	acc := seq.fn(seq.seq.First())
	cur := seq.seq.Rest()
	for !cur.IsEmpty() {
		acc = call2(f, acc, seq.fn(cur.First()))
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
		cur = cur.Rest()
	}
	return acc
}

func (seq *MappingSeq) ReduceInit(f coretypes.Callable, init Object) Object {
	acc := init
	cur := seq.seq
	for !cur.IsEmpty() {
		acc = call2(f, acc, seq.fn(cur.First()))
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
		cur = cur.Rest()
	}
	return acc
}

// ---- map_filter_fast.go ----
// map_filter_fast.go — AST-level fused reducible pipelines.
//
// Recognizes reduce over a map/filter/take pipeline rooted at range and executes
// it in one loop, avoiding lazy seq wrapper churn. This replaces the earlier
// square/even/take/range-only special case with a small general pipeline compiler.

type reducibleStepKind byte

const (
	reducibleMap reducibleStepKind = iota
	reducibleFilter
	reducibleTake
)

type reducibleIntrinsic byte

const (
	reducibleGeneric reducibleIntrinsic = iota
	reducibleSquareInt
	reducibleEvenInt
)

type reducibleStep struct {
	kind      reducibleStepKind
	intrinsic reducibleIntrinsic
	fn        coretypes.Callable
	takeLimit int
}

type reducibleRangePipeline struct {
	start int
	end   int
	step  int
	steps []reducibleStep
}

func evalReducePipelineFast(expr *CallExpr, env *LocalEnv) (Object, bool) {
	if !callableName(expr.callable, "reduce") || (len(expr.args) != 2 && len(expr.args) != 3) {
		return nil, false
	}

	reducerObj := Eval(expr.args[0], env)
	reducer, ok := reducerObj.(coretypes.Callable)
	if !ok {
		return nil, false
	}

	var init Object
	var collExpr Expr
	if len(expr.args) == 3 {
		init = Eval(expr.args[1], env)
		collExpr = expr.args[2]
	} else {
		collExpr = expr.args[1]
	}

	pipeline, ok := compileReducibleRangePipeline(collExpr, env)
	if !ok || pipeline.step == 0 || len(pipeline.steps) == 0 {
		return nil, false
	}

	if len(expr.args) == 2 {
		return reducePipelineNoInit(reducer, pipeline)
	}
	return reducePipelineInit(reducer, init, pipeline), true
}

func compileReducibleRangePipeline(expr Expr, env *LocalEnv) (reducibleRangePipeline, bool) {
	call, ok := expr.(*CallExpr)
	if !ok {
		return reducibleRangePipeline{}, false
	}

	if callableName(call.callable, "range") {
		start, end, step, ok := evalRangeArgs(call.args, env)
		if !ok || step == 0 {
			return reducibleRangePipeline{}, false
		}
		return reducibleRangePipeline{start: start, end: end, step: step}, true
	}

	if callableName(call.callable, "map") && len(call.args) == 2 {
		inner, ok := compileReducibleRangePipeline(call.args[1], env)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		if isSquareMapperExpr(call.args[0]) {
			inner.steps = append(inner.steps, reducibleStep{kind: reducibleMap, intrinsic: reducibleSquareInt})
			return inner, true
		}
		fnObj := Eval(call.args[0], env)
		fn, ok := fnObj.(coretypes.Callable)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		inner.steps = append(inner.steps, reducibleStep{kind: reducibleMap, fn: fn})
		return inner, true
	}

	if callableName(call.callable, "filter") && len(call.args) == 2 {
		inner, ok := compileReducibleRangePipeline(call.args[1], env)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		if callableName(call.args[0], "even?") {
			inner.steps = append(inner.steps, reducibleStep{kind: reducibleFilter, intrinsic: reducibleEvenInt})
			return inner, true
		}
		fnObj := Eval(call.args[0], env)
		fn, ok := fnObj.(coretypes.Callable)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		inner.steps = append(inner.steps, reducibleStep{kind: reducibleFilter, fn: fn})
		return inner, true
	}

	if callableName(call.callable, "take") && len(call.args) == 2 {
		inner, ok := compileReducibleRangePipeline(call.args[1], env)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		nObj := Eval(call.args[0], env)
		n, ok := nObj.(coretypes.Int)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		inner.steps = append(inner.steps, reducibleStep{kind: reducibleTake, takeLimit: n.I})
		return inner, true
	}

	return reducibleRangePipeline{}, false
}

func reducePipelineNoInit(reducer coretypes.Callable, p reducibleRangePipeline) (Object, bool) {
	seen := false
	var acc Object
	_, stopped := walkReducibleRangePipeline(p, func(v Object) bool {
		if !seen {
			acc = v
			seen = true
			return false
		}
		acc = reduceStepFast(reducer, acc, v)
		return IsReduced(acc)
	})
	if !seen {
		return call0(reducer), true
	}
	if stopped && IsReduced(acc) {
		return DerefReduced(acc), true
	}
	return acc, true
}

func reducePipelineInit(reducer coretypes.Callable, init Object, p reducibleRangePipeline) Object {
	acc := init
	reducerName := hotReducerName(reducer)
	_, stopped := walkReducibleRangePipeline(p, func(v Object) bool {
		acc = reduceStepFastByName(reducer, reducerName, acc, v)
		return IsReduced(acc)
	})
	if stopped && IsReduced(acc) {
		return DerefReduced(acc)
	}
	return acc
}

func walkReducibleRangePipeline(p reducibleRangePipeline, emit func(Object) bool) (emitted int, stopped bool) {
	takeRemaining := make([]int, len(p.steps))
	for i, step := range p.steps {
		if step.kind == reducibleTake {
			takeRemaining[i] = step.takeLimit
		}
	}

	for i := p.start; (p.step > 0 && i < p.end) || (p.step < 0 && i > p.end); i += p.step {
		v := Object(coretypes.Int{I: i})
		alive := true
		stopAfterCurrent := false

		for si, step := range p.steps {
			if !alive {
				break
			}
			switch step.kind {
			case reducibleMap:
				if step.intrinsic == reducibleSquareInt {
					if iv, ok := v.(coretypes.Int); ok {
						v = coretypes.Int{I: iv.I * iv.I}
					} else {
						v = call1(step.fn, v)
					}
				} else {
					v = call1(step.fn, v)
				}
			case reducibleFilter:
				if step.intrinsic == reducibleEvenInt {
					if iv, ok := v.(coretypes.Int); ok {
						alive = iv.I%2 == 0
					} else {
						alive = false
					}
				} else if !ToBool(call1(step.fn, v)) {
					alive = false
				}
			case reducibleTake:
				if takeRemaining[si] <= 0 {
					return emitted, true
				}
				takeRemaining[si]--
				if takeRemaining[si] == 0 {
					stopAfterCurrent = true
				}
			}
		}

		if alive {
			emitted++
			if emit(v) {
				return emitted, true
			}
		}
		if stopAfterCurrent {
			return emitted, true
		}
	}
	return emitted, false
}

func isSquareMapperExpr(expr Expr) bool {
	if fn, ok := expr.(*FnExpr); ok {
		return isSquareFnExpr(fn)
	}
	if le, ok := expr.(*LetExpr); ok {
		return isSquareFnExpr(extractFnExpr(le.body))
	}
	return false
}

func extractFnExpr(body []Expr) *FnExpr {
	if len(body) == 0 {
		return nil
	}
	switch e := body[len(body)-1].(type) {
	case *FnExpr:
		return e
	case *LetExpr:
		return extractFnExpr(e.body)
	case *DoExpr:
		return extractFnExpr(e.body)
	}
	return nil
}

func isSquareFnExpr(fn *FnExpr) bool {
	if fn == nil || len(fn.arities) != 1 || fn.variadic != nil {
		return false
	}
	arity := fn.arities[0]
	if len(arity.args) != 1 || len(arity.body) != 1 {
		return false
	}
	pf := guessFnParamFrame(arity.body, 1)
	if pf < 0 {
		pf = 1
	}
	call, ok := arity.body[0].(*CallExpr)
	if !ok || len(call.args) != 2 {
		return false
	}
	vref, ok := call.callable.(*VarRefExpr)
	if !ok || coreVarToProcName(vref.vr) != "procMultiply" {
		return false
	}
	lhs, lok := call.args[0].(*BindingExpr)
	rhs, rok := call.args[1].(*BindingExpr)
	return lok && rok && lhs.binding.frame == pf && rhs.binding.frame == pf && lhs.binding.index == 0 && rhs.binding.index == 0
}

func callableName(expr Expr, name string) bool {
	vref, ok := expr.(*VarRefExpr)
	return ok && vref.vr.name.ToString(false) == name
}

func evalRangeArgs(args []Expr, env *LocalEnv) (start, end, step int, ok bool) {
	switch len(args) {
	case 1:
		endObj := Eval(args[0], env)
		endInt, yes := endObj.(coretypes.Int)
		return 0, endInt.I, 1, yes
	case 2:
		startObj := Eval(args[0], env)
		endObj := Eval(args[1], env)
		startInt, sok := startObj.(coretypes.Int)
		endInt, eok := endObj.(coretypes.Int)
		return startInt.I, endInt.I, 1, sok && eok
	case 3:
		startObj := Eval(args[0], env)
		endObj := Eval(args[1], env)
		stepObj := Eval(args[2], env)
		startInt, sok := startObj.(coretypes.Int)
		endInt, eok := endObj.(coretypes.Int)
		stepInt, tok := stepObj.(coretypes.Int)
		return startInt.I, endInt.I, stepInt.I, sok && eok && tok
	}
	return 0, 0, 0, false
}

// ---- seq_ops_fast.go ----
// seq_ops_fast.go — Fast map/filter/take seq wrappers for reducible pipelines.

type FilteringSeq struct {
	coretypes.InfoHolder
	MetaHolder
	seq  Seq
	pred coretypes.Callable
}

type TakeSeq struct {
	coretypes.InfoHolder
	MetaHolder
	seq Seq
	n   int
}

func (s *FilteringSeq) ToString(escape bool) string   { return SeqToString(s, escape) }
func (s *FilteringSeq) Equals(other interface{}) bool { return IsSeqEqual(s, other) }
func (s *FilteringSeq) WithInfo(info *coretypes.ObjectInfo) Object {
	res := *s
	res.Info = info
	return &res
}
func (s *FilteringSeq) GetType() *coretypes.Type { return TYPE.LazySeq }
func (s *FilteringSeq) Hash() uint32             { return hashOrdered(s) }
func (s *FilteringSeq) WithMeta(m Map) Object {
	res := *s
	res.meta = SafeMerge(res.meta, m)
	return &res
}
func (s *FilteringSeq) Seq() Seq          { return s }
func (s *FilteringSeq) SequentialMarker() {}
func (s *FilteringSeq) IsEmpty() bool     { return s.nextSeq().IsEmpty() }
func (s *FilteringSeq) First() Object     { return s.nextSeq().First() }
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

func (s *FilteringSeq) Reduce(f coretypes.Callable) Object {
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

func (s *FilteringSeq) ReduceInit(f coretypes.Callable, init Object) Object {
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

func (s *TakeSeq) ToString(escape bool) string   { return SeqToString(s, escape) }
func (s *TakeSeq) Equals(other interface{}) bool { return IsSeqEqual(s, other) }
func (s *TakeSeq) WithInfo(info *coretypes.ObjectInfo) Object {
	res := *s
	res.Info = info
	return &res
}
func (s *TakeSeq) GetType() *coretypes.Type { return TYPE.LazySeq }
func (s *TakeSeq) Hash() uint32             { return hashOrdered(s) }
func (s *TakeSeq) WithMeta(m Map) Object    { res := *s; res.meta = SafeMerge(res.meta, m); return &res }
func (s *TakeSeq) Seq() Seq                 { return s }
func (s *TakeSeq) SequentialMarker()        {}
func (s *TakeSeq) IsEmpty() bool            { return s.n <= 0 || s.seq.IsEmpty() }
func (s *TakeSeq) First() Object            { return s.seq.First() }
func (s *TakeSeq) Rest() Seq {
	if s.n <= 1 {
		return EmptyList
	}
	return &TakeSeq{seq: s.seq.Rest(), n: s.n - 1}
}
func (s *TakeSeq) Cons(obj Object) Seq { return &ConsSeq{first: obj, rest: s} }

func (s *TakeSeq) Reduce(f coretypes.Callable) Object {
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

func (s *TakeSeq) reduceFused(f coretypes.Callable) (Object, bool) {
	return nil, false
}

func (s *TakeSeq) ReduceInit(f coretypes.Callable, init Object) Object {
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

func chunkedMapSeq(f coretypes.Callable, src Seq) Seq {
	if src == nil || src.IsEmpty() {
		return EmptyList
	}
	buf := make([]Object, 0, 32)
	cur := src
	for len(buf) < 32 && !cur.IsEmpty() {
		buf = append(buf, call1(f, cur.First()))
		cur = cur.Rest()
	}
	chunk := &ArrayChunk{arr: buf, off: 0, end: len(buf)}
	var rest Seq
	if !cur.IsEmpty() {
		restCur := cur
		rest = &LazySeq{fn: Proc{Name: "procChunkedMapRest", Fn: func(args []Object) Object {
			return chunkedMapSeq(f, restCur)
		}}}
	}
	return &ChunkedCons{chunk: chunk, rest: rest, idx: 0}
}

func chunkedFilterSeq(pred coretypes.Callable, src Seq) Seq {
	cur := src
	for {
		if cur == nil || cur.IsEmpty() {
			return EmptyList
		}
		buf := make([]Object, 0, 32)
		for len(buf) < 32 && !cur.IsEmpty() {
			v := cur.First()
			if ToBool(call1(pred, v)) {
				buf = append(buf, v)
			}
			cur = cur.Rest()
		}
		if len(buf) > 0 {
			chunk := &ArrayChunk{arr: buf, off: 0, end: len(buf)}
			var rest Seq
			if !cur.IsEmpty() {
				restCur := cur
				rest = &LazySeq{fn: Proc{Name: "procChunkedFilterRest", Fn: func(args []Object) Object {
					return chunkedFilterSeq(pred, restCur)
				}}}
			}
			return &ChunkedCons{chunk: chunk, rest: rest, idx: 0}
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

	mapVr.Value = Proc{Name: "procMapSeqFast", Fn: func(args []Object) Object {
		if len(args) == 1 {
			return makeMapTransducer(EnsureArgIsCallable(args, 0))
		}
		if len(args) == 2 {
			f := EnsureArgIsCallable(args, 0)
			s := EnsureObjectIsSeqable(args[1], "map requires seqable").Seq()
			if _, ok := s.(*ChunkedCons); ok {
				return chunkedMapSeq(f, s)
			}
			return &MappingSeq{seq: s, fn: func(o Object) Object { return call1(f, o) }}
		}
		return mapOrig.Call(args)
	}}
	filterVr.Value = Proc{Name: "procFilterSeqFast", Fn: func(args []Object) Object {
		if len(args) == 1 {
			return makeFilterTransducer(EnsureArgIsCallable(args, 0))
		}
		if len(args) == 2 {
			pred := EnsureArgIsCallable(args, 0)
			s := EnsureArgIsSeqable(args, 1).Seq()
			if _, ok := s.(*ChunkedCons); ok {
				return chunkedFilterSeq(pred, s)
			}
			return &FilteringSeq{seq: s, pred: pred}
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

// ---- frequencies_fast.go ----
// frequencies_fast.go — native fast path for core/frequencies.

func init() {
	vr := GLOBAL_ENV.CoreNamespace.Intern(MakeSymbol("frequencies"))
	vr.Value = Proc{Name: "procFrequencies", Fn: procFrequencies}
	referToUser(MakeSymbol("frequencies"), vr)

	sw := GLOBAL_ENV.CoreNamespace.Intern(MakeSymbol("split-whitespace__"))
	sw.Value = Proc{Name: "procSplitWhitespace", Fn: procSplitWhitespace}
	referToUser(MakeSymbol("split-whitespace"), sw)
}

var procSplitWhitespace ProcFn = func(args []Object) Object {
	CheckArity(args, 1, 1)
	return splitWhitespaceVector(EnsureArgIsString(args, 0).S)
}

func splitWhitespaceVector(s string) *ArrayVector {
	res := collectionConstruction.NewEmptyArrayVector()
	for _, token := range corestr.SplitWhitespace(s) {
		res.Append(coretypes.String{S: token})
	}
	return res
}

var procFrequencies ProcFn = func(args []Object) Object {
	CheckArity(args, 1, 1)
	seq := EnsureObjectIsSeqable(args[0], "frequencies requires a seqable collection").Seq()
	if seq.IsEmpty() {
		return collectionConstruction.NewEmptyArrayMap()
	}

	// Specialize the common text-token case: String keys and integer counts.
	// Avoids persistent map churn and repeated Object hash calculation in the
	// hot loop, then emits a normal persistent map at the boundary.
	stringCounts := make(map[string]int)
	var tm *TransientMap
	stringOnly := true
	for !seq.IsEmpty() {
		obj := seq.First()
		if stringOnly {
			if s, ok := obj.(coretypes.String); ok {
				stringCounts[s.S]++
				seq = seq.Rest()
				continue
			}
			stringOnly = false
			tm = MapToTransient(nil)
			for k, v := range stringCounts {
				tm.AssocInPlace(coretypes.String{S: k}, coretypes.Int{I: v})
			}
			stringCounts = nil
		}
		_, old := tm.Get(obj)
		cnt := 0
		if i, ok := old.(coretypes.Int); ok {
			cnt = i.I
		}
		tm.AssocInPlace(obj, coretypes.Int{I: cnt + 1})
		seq = seq.Rest()
	}
	if stringOnly {
		if len(stringCounts) <= int(HASHMAP_THRESHOLD/2) {
			res := collectionConstruction.NewEmptyArrayMap()
			for k, v := range stringCounts {
				res.Add(coretypes.String{S: k}, coretypes.Int{I: v})
			}
			return res
		}
		res := EmptyHashMap
		for k, v := range stringCounts {
			res = res.Assoc(coretypes.String{S: k}, coretypes.Int{I: v}).(*HashMap)
		}
		return res
	}
	return tm.ToPersistent()
}

// ---- range_fast.go ----
// range_fast.go — Efficient Range type that implements coretypes.Reduce for fast numeric reduce.

var hotReducerFnCache sync.Map // *Fn -> reducer proc name string

// IntRange represents a range of integers [start, end) with step.
type IntRange struct {
	coretypes.InfoHolder
	MetaHolder
	start, end, step int
}

func NewIntRange(start, end, step int) *IntRange {
	return &IntRange{start: start, end: end, step: step}
}

func (r *IntRange) ToString(escape bool) string             { return SeqToString(r.Seq(), escape) }
func (r *IntRange) Equals(other interface{}) bool           { return IsSeqEqual(r.Seq(), other) }
func (r *IntRange) WithInfo(i *coretypes.ObjectInfo) Object { res := *r; res.Info = i; return &res }
func (r *IntRange) GetType() *coretypes.Type                { return TYPE.LazySeq }
func (r *IntRange) Hash() uint32                            { return hashOrdered(r.Seq()) }
func (r *IntRange) WithMeta(m Map) Object                   { res := *r; res.meta = SafeMerge(res.meta, m); return &res }
func (r *IntRange) SequentialMarker()                       {}

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
		buf = append(buf, coretypes.Int{I: v})
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

func (r *IntRange) Reduce(f coretypes.Callable) Object {
	if r.isEmpty() {
		return f.Call(nil)
	}
	if result, ok := r.reduceFast(f); ok {
		return result
	}
	acc := Object(coretypes.Int{I: r.start})
	for i := r.start + r.step; r.contains(i); i += r.step {
		acc = call2(f, acc, coretypes.Int{I: i})
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
	}
	return acc
}

func (r *IntRange) ReduceInit(f coretypes.Callable, init Object) Object {
	if result, ok := r.reduceInitFast(f, init); ok {
		return result
	}
	acc := init
	for i := r.start; r.contains(i); i += r.step {
		acc = call2(f, acc, coretypes.Int{I: i})
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

func (r *IntRange) reduceFast(f coretypes.Callable) (Object, bool) {
	name := hotReducerName(f)
	switch name {
	case "procAdd", "procunchecked-add", "procunchecked-add-int":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			acc += i
		}
		return coretypes.Int{I: acc}, true
	case "procMultiply", "procunchecked-multiply", "procunchecked-multiply-int":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			acc *= i
		}
		return coretypes.Int{I: acc}, true
	case "procMax":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			if i > acc {
				acc = i
			}
		}
		return coretypes.Int{I: acc}, true
	case "procMin":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			if i < acc {
				acc = i
			}
		}
		return coretypes.Int{I: acc}, true
	}
	return nil, false
}

func (r *IntRange) reduceInitFast(f coretypes.Callable, init Object) (Object, bool) {
	if result, ok := r.reduceMapAssocFast(f, init); ok {
		return result, true
	}
	name := hotReducerName(f)
	switch acc := init.(type) {
	case coretypes.Int:
		v := acc.I
		switch name {
		case "procAdd", "procunchecked-add", "procunchecked-add-int":
			for i := r.start; r.contains(i); i += r.step {
				v += i
			}
			return coretypes.Int{I: v}, true
		case "procMultiply", "procunchecked-multiply", "procunchecked-multiply-int":
			for i := r.start; r.contains(i); i += r.step {
				v *= i
			}
			return coretypes.Int{I: v}, true
		case "procMax":
			for i := r.start; r.contains(i); i += r.step {
				if i > v {
					v = i
				}
			}
			return coretypes.Int{I: v}, true
		case "procMin":
			for i := r.start; r.contains(i); i += r.step {
				if i < v {
					v = i
				}
			}
			return coretypes.Int{I: v}, true
		}
	case coretypes.Double:
		v := acc.D
		switch name {
		case "procAdd":
			for i := r.start; r.contains(i); i += r.step {
				v += float64(i)
			}
			return coretypes.Double{D: v}, true
		case "procMultiply":
			for i := r.start; r.contains(i); i += r.step {
				v *= float64(i)
			}
			return coretypes.Double{D: v}, true
		case "procMax":
			for i := r.start; r.contains(i); i += r.step {
				fi := float64(i)
				if fi > v {
					v = fi
				}
			}
			return coretypes.Double{D: v}, true
		case "procMin":
			for i := r.start; r.contains(i); i += r.step {
				fi := float64(i)
				if fi < v {
					v = fi
				}
			}
			return coretypes.Double{D: v}, true
		}
	}
	return nil, false
}

func (r *IntRange) reduceMapAssocFast(f coretypes.Callable, init Object) (Object, bool) {
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
		tm.AssocInPlace(coretypes.Int{I: keyFn(0, i)}, coretypes.Int{I: valFn(0, i)})
	}
	return tm.ToPersistent(), true
}

func hotReducerName(f coretypes.Callable) string {
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

func findFnVarNameCallable(c coretypes.Callable) string {
	switch f := c.(type) {
	case *Fn:
		return findFnVarName(f)
	case *Var:
		return f.name.ToString(false)
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
	coretypes.InfoHolder
	MetaHolder
	r   *IntRange
	cur int
}

func (s *intRangeSeq) ToString(escape bool) string   { return SeqToString(s, escape) }
func (s *intRangeSeq) Equals(other interface{}) bool { return IsSeqEqual(s, other) }
func (s *intRangeSeq) WithInfo(info *coretypes.ObjectInfo) Object {
	res := *s
	res.Info = info
	return &res
}
func (s *intRangeSeq) GetType() *coretypes.Type { return TYPE.LazySeq }
func (s *intRangeSeq) Hash() uint32             { return hashOrdered(s) }
func (s *intRangeSeq) WithMeta(m Map) Object {
	res := *s
	res.meta = SafeMerge(res.meta, m)
	return &res
}
func (s *intRangeSeq) Seq() Seq          { return s }
func (s *intRangeSeq) SequentialMarker() {}
func (s *intRangeSeq) First() Object     { return coretypes.Int{I: s.cur} }
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
	origRange, ok := rangeVr.Value.(coretypes.Callable)
	if !ok {
		return
	}

	rangeVr.Value = Proc{Name: "procRangeFast", Fn: func(args []Object) Object {
		switch len(args) {
		case 1:
			if end, ok := args[0].(coretypes.Int); ok {
				return NewIntRange(0, end.I, 1)
			}
		case 2:
			if start, ok := args[0].(coretypes.Int); ok {
				if end, ok := args[1].(coretypes.Int); ok {
					return NewIntRange(start.I, end.I, 1)
				}
			}
		case 3:
			if start, ok := args[0].(coretypes.Int); ok {
				if end, ok := args[1].(coretypes.Int); ok {
					if step, ok := args[2].(coretypes.Int); ok && step.I != 0 {
						return NewIntRange(start.I, end.I, step.I)
					}
				}
			}
		}
		return origRange.Call(args)
	}}
}
