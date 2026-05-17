package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
)

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
	MetaHolder
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
func (xf *XForm) WithMeta(m Map) coretypes.Object {
	res := *xf
	res.meta = SafeMerge(res.meta, m)
	return &res
}

func (xf *XForm) Call(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	rf := EnsureArgIsCallable(args, 0)
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
					PanicArityMinMax(len(callArgs), 0, 2)
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
					PanicArityMinMax(len(callArgs), 0, 2)
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
					PanicArityMinMax(len(callArgs), 0, 2)
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
	rf := EnsureObjectIsCallable(rfObj, "transduce xform must produce a reducing function, got %s")

	s := EnsureObjectIsSeqable(collObj, "Arg of core/transduce must be coretypes.Seqable, got %s").Seq()
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
	rf := EnsureObjectIsCallable(reducingFnObj, "transduce reducing function must be coretypes.Callable, got %s")
	if r, ok := collObj.(*IntRange); ok {
		return transducePipelineRange(xf, rf, init, r)
	}
	s := EnsureObjectIsSeqable(collObj, "Arg of core/transduce must be coretypes.Seqable, got %s").Seq()
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

func referToUser(sym Symbol, vr *Var) {
	userNs := GLOBAL_ENV.FindNamespace(MakeSymbol("user"))
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
	reducedVr := ns.Intern(MakeSymbol("reduced"))
	reducedVr.Value = Proc{Name: "procReduced", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return MakeReduced(args[0])
	}}
	referToUser(MakeSymbol("reduced"), reducedVr)

	// reduced? — type check, no map lookup
	reducedQVr := ns.Intern(MakeSymbol("reduced?"))
	reducedQVr.Value = Proc{Name: "procReducedQ", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return coretypes.MakeBoolean(IsReduced(args[0]))
	}}
	referToUser(MakeSymbol("reduced?"), reducedQVr)

	// ensure-reduced
	ensureReducedVr := ns.Intern(MakeSymbol("ensure-reduced"))
	ensureReducedVr.Value = Proc{Name: "procEnsureReduced", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return EnsureReduced(args[0])
	}}
	referToUser(MakeSymbol("ensure-reduced"), ensureReducedVr)

	// unreduced — deref a Reduced box (identity if not reduced)
	unreducedVr := ns.Intern(MakeSymbol("unreduced"))
	unreducedVr.Value = Proc{Name: "procUnreduced", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return DerefReduced(args[0])
	}}
	referToUser(MakeSymbol("unreduced"), unreducedVr)

	// completing — wraps a reducing fn with optional completion step
	completingVr := ns.Intern(MakeSymbol("completing"))
	completingVr.Value = Proc{Name: "procCompleting", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) != 1 && len(args) != 2 {
			PanicArityMinMax(len(args), 1, 2)
		}
		f := EnsureArgIsCallable(args, 0)
		var cf coretypes.Callable
		if len(args) == 2 {
			cf = EnsureArgIsCallable(args, 1)
		} else {
			cf = Proc{Name: "procCompletingIdentity", Fn: func(callArgs []coretypes.Object) coretypes.Object {
				CheckArity(callArgs, 1, 1)
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
				PanicArityMinMax(len(callArgs), 0, 2)
				return NIL
			}
		}}
	}}
	referToUser(MakeSymbol("completing"), completingVr)

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
			f := EnsureArgIsCallable(args, 0)
			return makeMapTransducer(f)
		}
		return mapOrig.Call(args)
	}}

	// filter transducer arity: (filter pred) returns a transducer
	filterVr.Value = Proc{Name: "procFilterXfCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			pred := EnsureArgIsCallable(args, 0)
			return makeFilterTransducer(pred)
		}
		return filterOrig.Call(args)
	}}

	// take transducer arity: (take n) returns a transducer when used with transduce
	takeVr.Value = Proc{Name: "procTakeXfCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			n := EnsureArgIsNumber(args, 0).Int().I
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
			PanicArityMinMax(len(args), 3, 4)
		}

		xform := EnsureArgIsCallable(args, 0)
		reducingFnObj := args[1]
		f := EnsureArgIsCallable(args, 1)

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
	transduceVr := ns.Intern(MakeSymbol("transduce"))
	transduceVr.Value = transduceProc
	referToUser(MakeSymbol("transduce"), transduceVr)

	// eduction — materializes transducer pipeline into a vector
	eductionVr := ns.Intern(MakeSymbol("eduction"))
	eductionVr.Value = Proc{Name: "procEduction", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			PanicArityMinMax(len(args), 2, 999)
		}
		collObj := args[len(args)-1]
		var xformObj coretypes.Object
		if len(args) == 2 {
			xformObj = args[0]
		} else {
			compVr := ns.Resolve("comp")
			if compVr == nil {
				panic(RT.NewError("Unable to resolve core/comp for eduction"))
			}
			compFn := EnsureObjectIsCallable(compVr.Value, "comp must be callable, got %s")
			xformObj = compFn.Call(args[:len(args)-1])
		}
		xform := EnsureObjectIsCallable(xformObj, "eduction expected callable xform, got %s")

		conjRF := Proc{Name: "procEductionConjRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
			switch len(callArgs) {
			case 0:
				return collectionConstruction.NewEmptyArrayVector()
			case 1:
				return callArgs[0]
			case 2:
				acc, ok := callArgs[0].(coretypes.Conjable)
				if !ok {
					panic(FailArg(callArgs[0], "coretypes.Conjable", 0))
				}
				return acc.Conj(callArgs[1]).(coretypes.Object)
			default:
				PanicArityMinMax(len(callArgs), 0, 2)
				return NIL
			}
		}}

		return transduceInternal(xform, conjRF, collectionConstruction.NewEmptyArrayVector(), collObj)
	}}
	referToUser(MakeSymbol("eduction"), eductionVr)

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
