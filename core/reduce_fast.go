package core

import (
	"sync"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

// ---- reduce_fast.go ----
// reduce_fast.go — coretypes.Seq-walking reduce fallback + IntRange creation at reduce time.

func seqReduceInit(s coretypes.Seq, f coretypes.Callable, init coretypes.Object) coretypes.Object {
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

func seqReduce(s coretypes.Seq, f coretypes.Callable) coretypes.Object {
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

func evalReducePipelineFast(expr *CallExpr, env *LocalEnv) (coretypes.Object, bool) {
	if !callableName(expr.callable, "reduce") || (len(expr.args) != 2 && len(expr.args) != 3) {
		return nil, false
	}

	reducerObj := Eval(expr.args[0], env)
	reducer, ok := reducerObj.(coretypes.Callable)
	if !ok {
		return nil, false
	}

	var init coretypes.Object
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

func reducePipelineNoInit(reducer coretypes.Callable, p reducibleRangePipeline) (coretypes.Object, bool) {
	seen := false
	var acc coretypes.Object
	_, stopped := walkReducibleRangePipeline(p, func(v coretypes.Object) bool {
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

func reducePipelineInit(reducer coretypes.Callable, init coretypes.Object, p reducibleRangePipeline) coretypes.Object {
	acc := init
	reducerName := hotReducerName(reducer)
	_, stopped := walkReducibleRangePipeline(p, func(v coretypes.Object) bool {
		acc = reduceStepFastByName(reducer, reducerName, acc, v)
		return IsReduced(acc)
	})
	if stopped && IsReduced(acc) {
		return DerefReduced(acc)
	}
	return acc
}

func walkReducibleRangePipeline(p reducibleRangePipeline, emit func(coretypes.Object) bool) (emitted int, stopped bool) {
	takeRemaining := make([]int, len(p.steps))
	for i, step := range p.steps {
		if step.kind == reducibleTake {
			takeRemaining[i] = step.takeLimit
		}
	}

	for i := p.start; (p.step > 0 && i < p.end) || (p.step < 0 && i > p.end); i += p.step {
		v := coretypes.Object(coretypes.Int{I: i})
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
		if ToBool(call1(s.pred, cur.First())) {
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

func (s *FilteringSeq) ReduceInit(f coretypes.Callable, init coretypes.Object) coretypes.Object {
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
		if IsReduced(acc) {
			return DerefReduced(acc)
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
		if IsReduced(acc) {
			return DerefReduced(acc)
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
			if ToBool(call1(pred, v)) {
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

// ---- frequencies_fast.go ----
// frequencies_fast.go — native fast path for core/frequencies.

func init() {
	vr := GLOBAL_ENV.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "frequencies"))
	vr.Value = Proc{Name: "procFrequencies", Fn: procFrequencies}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "frequencies"), vr)

	sw := GLOBAL_ENV.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "split-whitespace__"))
	sw.Value = Proc{Name: "procSplitWhitespace", Fn: procSplitWhitespace}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "split-whitespace"), sw)
}

var procSplitWhitespace ProcFn = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	return splitWhitespaceVector(coretypes.EnsureArgIsString(args, 0).S)
}

func splitWhitespaceVector(s string) *corecollections.ArrayVector {
	res := corecollections.EmptyArrayVector()
	for _, token := range corestr.SplitWhitespace(s) {
		res.Append(coretypes.String{S: token})
	}
	return res
}

var procFrequencies ProcFn = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	seq := coretypes.EnsureObjectIsSeqable(args[0], "frequencies requires a seqable collection").Seq()
	if seq.IsEmpty() {
		return corecollections.EmptyArrayMap()
	}

	// Specialize the common text-token case: String keys and integer counts.
	// Avoids persistent map churn and repeated coretypes.Object hash calculation in the
	// hot loop, then emits a normal persistent map at the boundary.
	stringCounts := make(map[string]int)
	var tm *coretypes.TransientMap
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
			tm = coretypes.MapToTransient(nil)
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
		if len(stringCounts) <= int(corecollections.HASHMAP_THRESHOLD/2) {
			res := corecollections.EmptyArrayMap()
			for k, v := range stringCounts {
				res.Add(coretypes.String{S: k}, coretypes.Int{I: v})
			}
			return res
		}
		res := corecollections.EmptyHashMap
		for k, v := range stringCounts {
			res = res.Assoc(coretypes.String{S: k}, coretypes.Int{I: v}).(*corecollections.HashMap)
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
	coretypes.MetaHolder
	start, end, step int
}

func NewIntRange(start, end, step int) *IntRange {
	return &IntRange{start: start, end: end, step: step}
}

func (r *IntRange) ToString(escape bool) string {
	return corecollections.SeqToString(r.Seq(), func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (r *IntRange) Equals(other interface{}) bool { return coretypes.IsSeqEqual(r.Seq(), other) }
func (r *IntRange) WithInfo(i *coretypes.ObjectInfo) coretypes.Object {
	res := *r
	res.Info = i
	return &res
}
func (r *IntRange) GetType() *coretypes.Type { return TYPE.LazySeq }
func (r *IntRange) Hash() uint32             { return corecollections.HashOrdered(r.Seq()) }
func (r *IntRange) WithMeta(m coretypes.Map) coretypes.Object {
	res := *r
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}
func (r *IntRange) SequentialMarker() {}

func (r *IntRange) Seq() coretypes.Seq {
	if r.step > 0 && r.start >= r.end {
		return corecollections.EmptyList
	}
	if r.step < 0 && r.start <= r.end {
		return corecollections.EmptyList
	}
	return r.chunkedSeqFrom(r.start)
}

func (r *IntRange) chunkedSeqFrom(cur int) coretypes.Seq {
	if !r.contains(cur) {
		return corecollections.EmptyList
	}
	buf := make([]coretypes.Object, 0, 32)
	v := cur
	for len(buf) < 32 && r.contains(v) {
		buf = append(buf, coretypes.Int{I: v})
		v += r.step
	}
	chunk := &corecollections.ArrayChunk{Arr: buf, Off: 0, End: len(buf)}
	var rest coretypes.Seq
	if r.contains(v) {
		rest = &corecollections.LazySeq{Fn: Proc{Name: "procIntRangeChunkRest", Fn: func(args []coretypes.Object) coretypes.Object {
			return r.chunkedSeqFrom(v)
		}}}
	}
	return &corecollections.ChunkedCons{Chunk: chunk, RestSeq: rest, Idx: 0}
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
		panic(coretypes.RuntimeError("range: step must not be 0"))
	}
	return 0
}

func (r *IntRange) Reduce(f coretypes.Callable) coretypes.Object {
	if r.isEmpty() {
		return f.Call(nil)
	}
	if result, ok := r.reduceFast(f); ok {
		return result
	}
	acc := coretypes.Object(coretypes.Int{I: r.start})
	for i := r.start + r.step; r.contains(i); i += r.step {
		acc = call2(f, acc, coretypes.Int{I: i})
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
	}
	return acc
}

func (r *IntRange) ReduceInit(f coretypes.Callable, init coretypes.Object) coretypes.Object {
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

func (r *IntRange) reduceFast(f coretypes.Callable) (coretypes.Object, bool) {
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

func (r *IntRange) reduceInitFast(f coretypes.Callable, init coretypes.Object) (coretypes.Object, bool) {
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

func (r *IntRange) reduceMapAssocFast(f coretypes.Callable, init coretypes.Object) (coretypes.Object, bool) {
	fn, ok := f.(*Fn)
	if !ok || fn == nil || fn.fnExpr == nil || len(fn.fnExpr.arities) != 1 || fn.fnExpr.variadic != nil {
		return nil, false
	}
	m, ok := init.(coretypes.Map)
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
	tm := coretypes.MapToTransient(m)
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
	coretypes.MetaHolder
	r   *IntRange
	cur int
}

func (s *intRangeSeq) ToString(escape bool) string {
	return corecollections.SeqToString(s, func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (s *intRangeSeq) Equals(other interface{}) bool { return coretypes.IsSeqEqual(s, other) }
func (s *intRangeSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *s
	res.Info = info
	return &res
}
func (s *intRangeSeq) GetType() *coretypes.Type { return TYPE.LazySeq }
func (s *intRangeSeq) Hash() uint32             { return corecollections.HashOrdered(s) }
func (s *intRangeSeq) WithMeta(m coretypes.Map) coretypes.Object {
	res := *s
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}
func (s *intRangeSeq) Seq() coretypes.Seq      { return s }
func (s *intRangeSeq) SequentialMarker()       {}
func (s *intRangeSeq) First() coretypes.Object { return coretypes.Int{I: s.cur} }
func (s *intRangeSeq) Rest() coretypes.Seq {
	next := s.cur + s.r.step
	if s.r.step > 0 && next >= s.r.end {
		return corecollections.EmptyList
	}
	if s.r.step < 0 && next <= s.r.end {
		return corecollections.EmptyList
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
func (s *intRangeSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &corecollections.ConsSeq{FirstValue: obj, RestValue: s}
}

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

	rangeVr.Value = Proc{Name: "procRangeFast", Fn: func(args []coretypes.Object) coretypes.Object {
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

// ---- transducer_compat.go ----

// transducer_compat.go — Transducer runtime support with proper Reduced type.
//
// Provides full Clojure transducer semantics:
// - transducer arities for map/filter/take
// - transduce (3-arity and 4-arity)
// - reduced, reduced?, ensure-reduced, unreduced
// - completing (1 and 2-arity)
// - eduction (materialized vector-backed)
// - sequence 2-arity via eduction

// xformKind describes one compiled transducer step.
type xformKind byte

const (
	xformMap xformKind = iota
	xformFilter
	xformTake
)

type xformStep struct {
	kind      xformKind
	intrinsic reducibleIntrinsic
	fn        coretypes.Callable
	n         int
}

// XForm is an internal transducer pipeline representation.
// It is also coretypes.Callable, so it remains compatible with generic transducer use.
type XForm struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	steps []xformStep
}

func (xf *XForm) ToString(escape bool) string   { return "#object[XForm]" }
func (xf *XForm) Equals(other interface{}) bool { return xf == other }
func (xf *XForm) GetType() *coretypes.Type      { return TYPE.Fn }
func (xf *XForm) Hash() uint32                  { return hashutil.Ptr(uintptr(unsafe.Pointer(xf))) }
func (xf *XForm) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *xf
	res.Info = info
	return &res
}
func (xf *XForm) WithMeta(m coretypes.Map) coretypes.Object {
	res := *xf
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}

func (xf *XForm) Call(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	rf := coretypes.EnsureArgIsCallable(args, 0)
	return buildXFormRF(xf.steps, rf).(coretypes.Object)
}

func buildXFormRF(steps []xformStep, rf coretypes.Callable) coretypes.Callable {
	wrapped := rf
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		switch step.kind {
		case xformMap:
			f := step.fn
			downstream := wrapped
			wrapped = Proc{Name: "procMapXfRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
				switch len(callArgs) {
				case 0:
					return call0(downstream)
				case 1:
					return downstream.Call(callArgs)
				case 2:
					return call2(downstream, callArgs[0], call1(f, callArgs[1]))
				default:
					coretypes.RuntimePanicArityMinMax(len(callArgs), 0, 2)
					return NIL
				}
			}}
		case xformFilter:
			pred := step.fn
			downstream := wrapped
			wrapped = Proc{Name: "procFilterXfRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
				switch len(callArgs) {
				case 0:
					return call0(downstream)
				case 1:
					return downstream.Call(callArgs)
				case 2:
					if ToBool(call1(pred, callArgs[1])) {
						return downstream.Call(callArgs)
					}
					return callArgs[0]
				default:
					coretypes.RuntimePanicArityMinMax(len(callArgs), 0, 2)
					return NIL
				}
			}}
		case xformTake:
			limit := step.n
			downstream := wrapped
			remaining := limit
			wrapped = Proc{Name: "procTakeXfRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
				switch len(callArgs) {
				case 0:
					return call0(downstream)
				case 1:
					return downstream.Call(callArgs)
				case 2:
					if remaining <= 0 {
						return EnsureReduced(callArgs[0])
					}
					out := downstream.Call(callArgs)
					remaining--
					if remaining <= 0 {
						return EnsureReduced(out)
					}
					return out
				default:
					coretypes.RuntimePanicArityMinMax(len(callArgs), 0, 2)
					return NIL
				}
			}}
		}
	}
	return wrapped
}

func completeReducingFn(rf coretypes.Callable, res coretypes.Object) coretypes.Object {
	completed := res
	func() {
		defer func() {
			if recover() != nil {
				completed = res
			}
		}()
		completed = call1(rf, res)
	}()
	return DerefReduced(completed)
}

func transduceInternal(xform coretypes.Callable, reducingFnObj coretypes.Object, init coretypes.Object, collObj coretypes.Object) coretypes.Object {
	if xf, ok := xform.(*XForm); ok {
		return transducePipeline(xf, reducingFnObj, init, collObj)
	}
	rfObj := call1(xform, reducingFnObj)
	rf := coretypes.EnsureObjectIsCallable(rfObj, "transduce xform must produce a reducing function, got %s")

	s := coretypes.EnsureObjectIsSeqable(collObj, "Arg of core/transduce must be coretypes.Seqable, got %s").Seq()
	res := init
	for !s.IsEmpty() {
		step := call2(rf, res, s.First())
		if IsReduced(step) {
			res = DerefReduced(step)
			return completeReducingFn(rf, res)
		}
		res = step
		s = s.Rest()
	}
	return completeReducingFn(rf, res)
}

func transducePipeline(xf *XForm, reducingFnObj coretypes.Object, init coretypes.Object, collObj coretypes.Object) coretypes.Object {
	rf := coretypes.EnsureObjectIsCallable(reducingFnObj, "transduce reducing function must be coretypes.Callable, got %s")
	if r, ok := collObj.(*IntRange); ok {
		return transducePipelineRange(xf, rf, init, r)
	}
	s := coretypes.EnsureObjectIsSeqable(collObj, "Arg of core/transduce must be coretypes.Seqable, got %s").Seq()
	res := init
	reducerName := hotReducerName(rf)
	takeRemaining := -1
	for _, step := range xf.steps {
		if step.kind == xformTake {
			takeRemaining = step.n
			break
		}
	}
	for !s.IsEmpty() {
		val := s.First()
		include := true
		stopAfter := false
		for _, step := range xf.steps {
			switch step.kind {
			case xformMap:
				val = applyXFormMapStep(step, val)
			case xformFilter:
				if !applyXFormFilterStep(step, val) {
					include = false
				}
			case xformTake:
				if takeRemaining <= 0 {
					return completeReducingFn(rf, res)
				}
				takeRemaining--
				if takeRemaining == 0 {
					stopAfter = true
				}
			}
			if !include {
				break
			}
		}
		if include {
			step := reduceStepFastByName(rf, reducerName, res, val)
			if IsReduced(step) {
				return completeReducingFn(rf, DerefReduced(step))
			}
			res = step
			if stopAfter {
				return completeReducingFn(rf, res)
			}
		}
		s = s.Rest()
	}
	return completeReducingFn(rf, res)
}

func transducePipelineRange(xf *XForm, rf coretypes.Callable, init coretypes.Object, r *IntRange) coretypes.Object {
	res := init
	reducerName := hotReducerName(rf)
	takeRemaining := -1
	for _, step := range xf.steps {
		if step.kind == xformTake {
			takeRemaining = step.n
			break
		}
	}
	for i := r.start; r.contains(i); i += r.step {
		val := coretypes.Object(coretypes.Int{I: i})
		include := true
		stopAfter := false
		for _, step := range xf.steps {
			switch step.kind {
			case xformMap:
				val = applyXFormMapStep(step, val)
			case xformFilter:
				if !applyXFormFilterStep(step, val) {
					include = false
				}
			case xformTake:
				if takeRemaining <= 0 {
					return completeReducingFn(rf, res)
				}
				takeRemaining--
				if takeRemaining == 0 {
					stopAfter = true
				}
			}
			if !include {
				break
			}
		}
		if include {
			step := reduceStepFastByName(rf, reducerName, res, val)
			if IsReduced(step) {
				return completeReducingFn(rf, DerefReduced(step))
			}
			res = step
			if stopAfter {
				return completeReducingFn(rf, res)
			}
		}
	}
	return completeReducingFn(rf, res)
}

func applyXFormMapStep(step xformStep, val coretypes.Object) coretypes.Object {
	if step.intrinsic == reducibleSquareInt {
		if iv, ok := val.(coretypes.Int); ok {
			return coretypes.Int{I: iv.I * iv.I}
		}
	}
	return call1(step.fn, val)
}

func applyXFormFilterStep(step xformStep, val coretypes.Object) bool {
	if step.intrinsic == reducibleEvenInt {
		if iv, ok := val.(coretypes.Int); ok {
			return iv.I%2 == 0
		}
		return false
	}
	return ToBool(call1(step.fn, val))
}

func reduceStepFast(rf coretypes.Callable, acc coretypes.Object, val coretypes.Object) coretypes.Object {
	return reduceStepFastByName(rf, hotReducerName(rf), acc, val)
}

func reduceStepFastByName(rf coretypes.Callable, name string, acc coretypes.Object, val coretypes.Object) coretypes.Object {
	switch name {
	case "procAdd", "procunchecked-add", "procunchecked-add-int":
		if a, ok := acc.(coretypes.Int); ok {
			if b, ok := val.(coretypes.Int); ok {
				return coretypes.Int{I: a.I + b.I}
			}
		}
	case "procMultiply", "procunchecked-multiply", "procunchecked-multiply-int":
		if a, ok := acc.(coretypes.Int); ok {
			if b, ok := val.(coretypes.Int); ok {
				return coretypes.Int{I: a.I * b.I}
			}
		}
	}
	return call2(rf, acc, val)
}

func makeMapTransducer(f coretypes.Callable) coretypes.Object {
	step := xformStep{kind: xformMap, fn: f}
	if fn, ok := f.(*Fn); ok && isSquareFnExpr(fn.fnExpr) {
		step.intrinsic = reducibleSquareInt
	}
	return &XForm{steps: []xformStep{step}}
}

func makeFilterTransducer(pred coretypes.Callable) coretypes.Object {
	step := xformStep{kind: xformFilter, fn: pred}
	if findFnVarNameCallable(pred) == "even?" {
		step.intrinsic = reducibleEvenInt
	}
	return &XForm{steps: []xformStep{step}}
}

func makeTakeTransducer(n int) coretypes.Object {
	if n < 0 {
		n = 0
	}
	return &XForm{steps: []xformStep{{kind: xformTake, n: n}}}
}

func referToUser(sym coretypes.Symbol, vr *Var) {
	userNs := GLOBAL_ENV.FindNamespace(coretypes.MakeSymbol(STRINGS.Intern, "user"))
	if userNs != nil {
		userNs.Refer(sym, vr)
	}
}

func installTransducerCompat() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// Fix reduce-kv to handle nil coll (returns init)
	rkvVr := ns.Resolve("reduce-kv")
	if rkvVr != nil {
		origRKV, ok := rkvVr.Value.(coretypes.Callable)
		if ok {
			rkvVr.Value = Proc{Name: "procReduceKvNilSafe", Fn: func(args []coretypes.Object) coretypes.Object {
				if len(args) >= 3 {
					coll := args[2]
					if coll == nil {
						return args[1]
					}
					if _, ok := coll.(Nil); ok {
						return args[1]
					}
				}
				return origRKV.Call(args)
			}}
		}
	}

	// reduced — wraps value in Reduced box
	reducedVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "reduced"))
	reducedVr.Value = Proc{Name: "procReduced", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return MakeReduced(args[0])
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "reduced"), reducedVr)

	// reduced? — type check, no map lookup
	reducedQVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "reduced?"))
	reducedQVr.Value = Proc{Name: "procReducedQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return coretypes.MakeBoolean(IsReduced(args[0]))
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "reduced?"), reducedQVr)

	// ensure-reduced
	ensureReducedVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "ensure-reduced"))
	ensureReducedVr.Value = Proc{Name: "procEnsureReduced", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return EnsureReduced(args[0])
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "ensure-reduced"), ensureReducedVr)

	// unreduced — deref a Reduced box (identity if not reduced)
	unreducedVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "unreduced"))
	unreducedVr.Value = Proc{Name: "procUnreduced", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return DerefReduced(args[0])
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "unreduced"), unreducedVr)

	// completing — wraps a reducing fn with optional completion step
	completingVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "completing"))
	completingVr.Value = Proc{Name: "procCompleting", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) != 1 && len(args) != 2 {
			coretypes.RuntimePanicArityMinMax(len(args), 1, 2)
		}
		f := coretypes.EnsureArgIsCallable(args, 0)
		var cf coretypes.Callable
		if len(args) == 2 {
			cf = coretypes.EnsureArgIsCallable(args, 1)
		} else {
			cf = Proc{Name: "procCompletingIdentity", Fn: func(callArgs []coretypes.Object) coretypes.Object {
				runtimeCheckArity(callArgs, 1, 1)
				return callArgs[0]
			}}
		}
		return Proc{Name: "procCompletingRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
			switch len(callArgs) {
			case 0:
				return f.Call(nil)
			case 1:
				return cf.Call(callArgs)
			case 2:
				return f.Call(callArgs)
			default:
				coretypes.RuntimePanicArityMinMax(len(callArgs), 0, 2)
				return NIL
			}
		}}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "completing"), completingVr)

	mapVr := ns.Resolve("map")
	filterVr := ns.Resolve("filter")
	takeVr := ns.Resolve("take")
	sequenceVr := ns.Resolve("sequence")
	compVr := ns.Resolve("comp")
	if mapVr == nil || filterVr == nil || takeVr == nil || sequenceVr == nil || compVr == nil {
		return
	}

	mapOrig, mapOK := mapVr.Value.(coretypes.Callable)
	filterOrig, filterOK := filterVr.Value.(coretypes.Callable)
	takeOrig, takeOK := takeVr.Value.(coretypes.Callable)
	sequenceOrig, sequenceOK := sequenceVr.Value.(coretypes.Callable)
	compOrig, compOK := compVr.Value.(coretypes.Callable)
	if !mapOK || !filterOK || !takeOK || !sequenceOK || !compOK {
		return
	}

	// map transducer arity: (map f) returns a transducer
	mapVr.Value = Proc{Name: "procMapXfCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			f := coretypes.EnsureArgIsCallable(args, 0)
			return makeMapTransducer(f)
		}
		return mapOrig.Call(args)
	}}

	// filter transducer arity: (filter pred) returns a transducer
	filterVr.Value = Proc{Name: "procFilterXfCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			pred := coretypes.EnsureArgIsCallable(args, 0)
			return makeFilterTransducer(pred)
		}
		return filterOrig.Call(args)
	}}

	// take transducer arity: (take n) returns a transducer when used with transduce
	takeVr.Value = Proc{Name: "procTakeXfCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			n := coretypes.EnsureArgIsNumber(args, 0).Int().I
			return makeTakeTransducer(n)
		}
		return takeOrig.Call(args)
	}}

	// comp of internal xforms returns a fused pipeline.
	compVr.Value = Proc{Name: "procCompXfCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) > 0 {
			steps := make([]xformStep, 0)
			for _, arg := range args {
				xf, ok := arg.(*XForm)
				if !ok {
					return compOrig.Call(args)
				}
				steps = append(steps, xf.steps...)
			}
			return &XForm{steps: steps}
		}
		return compOrig.Call(args)
	}}

	// transduce — full 3 and 4-arity support
	transduceProc := Proc{Name: "procTransduce", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) != 3 && len(args) != 4 {
			coretypes.RuntimePanicArityMinMax(len(args), 3, 4)
		}

		xform := coretypes.EnsureArgIsCallable(args, 0)
		reducingFnObj := args[1]
		f := coretypes.EnsureArgIsCallable(args, 1)

		var init coretypes.Object
		var collObj coretypes.Object
		if len(args) == 4 {
			init = args[2]
			collObj = args[3]
		} else {
			init = f.Call(nil)
			collObj = args[2]
		}

		return transduceInternal(xform, reducingFnObj, init, collObj)
	}}
	transduceVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "transduce"))
	transduceVr.Value = transduceProc
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "transduce"), transduceVr)

	// eduction — materializes transducer pipeline into a vector
	eductionVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "eduction"))
	eductionVr.Value = Proc{Name: "procEduction", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			coretypes.RuntimePanicArityMinMax(len(args), 2, 999)
		}
		collObj := args[len(args)-1]
		var xformObj coretypes.Object
		if len(args) == 2 {
			xformObj = args[0]
		} else {
			compVr := ns.Resolve("comp")
			if compVr == nil {
				panic(coretypes.RuntimeError("Unable to resolve core/comp for eduction"))
			}
			compFn := coretypes.EnsureObjectIsCallable(compVr.Value, "comp must be callable, got %s")
			xformObj = compFn.Call(args[:len(args)-1])
		}
		xform := coretypes.EnsureObjectIsCallable(xformObj, "eduction expected callable xform, got %s")

		conjRF := Proc{Name: "procEductionConjRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
			switch len(callArgs) {
			case 0:
				return corecollections.EmptyArrayVector()
			case 1:
				return callArgs[0]
			case 2:
				acc, ok := callArgs[0].(coretypes.Conjable)
				if !ok {
					panic(FailArg(callArgs[0], "coretypes.Conjable", 0))
				}
				return acc.Conj(callArgs[1]).(coretypes.Object)
			default:
				coretypes.RuntimePanicArityMinMax(len(callArgs), 0, 2)
				return NIL
			}
		}}

		return transduceInternal(xform, conjRF, corecollections.EmptyArrayVector(), collObj)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "eduction"), eductionVr)

	// sequence 2-arity: (sequence xform coll) → lazy seq of eduction result
	sequenceVr.Value = Proc{Name: "procSequenceCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 2 {
			res := eductionVr.Value.(coretypes.Callable).Call(args)
			if s, ok := res.(coretypes.Seqable); ok {
				return s.Seq()
			}
			return NIL
		}
		return sequenceOrig.Call(args)
	}}
}

func init() {
	installTransducerCompat()
	maybeOverrideRange()
}

// ---- reduced.go ----
// reduced.go — Proper Reduced type for transducer early termination.
//
// In Clojure, (reduced x) wraps x in a Reduced box that signals
// early termination to reduce/transduce. This replaces the corecollections.ArrayMap-based
// shim with a proper type that's fast to create, check, and unwrap.

// Reduced wraps a value to signal early termination in reduce/transduce.
type Reduced struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	Val coretypes.Object
}

func (r *Reduced) ToString(escape bool) string {
	return "#object[Reduced " + r.Val.ToString(escape) + "]"
}

func (r *Reduced) Equals(other interface{}) bool {
	if o, ok := other.(*Reduced); ok {
		return r.Val.Equals(o.Val)
	}
	return false
}

func (r *Reduced) GetType() *coretypes.Type {
	return TYPE.Fn // reuse Fn type slot for now
}

func (r *Reduced) Hash() uint32 {
	return r.Val.Hash() ^ 0xDEADBEEF
}

func (r *Reduced) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *r
	res.Info = info
	return &res
}

func (r *Reduced) WithMeta(m coretypes.Map) coretypes.Object {
	res := *r
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}

// MakeReduced wraps a value in a Reduced box.
func MakeReduced(val coretypes.Object) *Reduced {
	return &Reduced{Val: val}
}

// IsReduced checks if an object is a Reduced box (type assertion, no map lookup).
func IsReduced(obj coretypes.Object) bool {
	_, ok := obj.(*Reduced)
	return ok
}

// DerefReduced unwraps a Reduced box, returning the inner value.
// If not reduced, returns the value as-is.
func DerefReduced(obj coretypes.Object) coretypes.Object {
	if r, ok := obj.(*Reduced); ok {
		return r.Val
	}
	return obj
}

// EnsureReduced wraps a value in Reduced if it isn't already.
func EnsureReduced(obj coretypes.Object) *Reduced {
	if r, ok := obj.(*Reduced); ok {
		return r
	}
	return MakeReduced(obj)
}
