package core

// Runtime compatibility shims for transducer-style workloads and basic
// Clojure transducer helpers.
//
// Provides:
// - transducer arities for map/filter/take
// - transduce
// - reduced, reduced?, ensure-reduced, unreduced
// - completing
// - eduction (materialized vector-backed)
// - sequence 2-arity via eduction

var (
	transducerReducedKey = MakeKeyword("joker.core/reduced")
	transducerValueKey   = MakeKeyword("value")
)

func reducedWrap(v Object) Object {
	m := EmptyArrayMap()
	m.Add(transducerReducedKey, Boolean{B: true})
	m.Add(transducerValueKey, v)
	return m
}

func isReducedObj(v Object) bool {
	m, ok := v.(Gettable)
	if !ok {
		return false
	}
	ok, tag := m.Get(transducerReducedKey)
	return ok && ToBool(tag)
}

func unreducedObj(v Object) Object {
	m, ok := v.(Gettable)
	if !ok {
		return v
	}
	ok, tag := m.Get(transducerReducedKey)
	if !ok || !ToBool(tag) {
		return v
	}
	if ok, inner := m.Get(transducerValueKey); ok {
		return inner
	}
	return NIL
}

func ensureReducedObj(v Object) Object {
	if isReducedObj(v) {
		return v
	}
	return reducedWrap(v)
}

func completeReducingFn(rf Callable, res Object) Object {
	completed := res
	func() {
		defer func() {
			if recover() != nil {
				completed = res
			}
		}()
		completed = rf.Call([]Object{res})
	}()
	if isReducedObj(completed) {
		return unreducedObj(completed)
	}
	return completed
}

func transduceInternal(xform Callable, reducingFnObj Object, init Object, collObj Object) Object {
	rfObj := xform.Call([]Object{reducingFnObj})
	rf := EnsureObjectIsCallable(rfObj, "transduce xform must produce a reducing function, got %s")

	s := EnsureObjectIsSeqable(collObj, "Arg of core/transduce must be Seqable, got %s").Seq()
	res := init
	for !s.IsEmpty() {
		step := rf.Call([]Object{res, s.First()})
		if isReducedObj(step) {
			res = unreducedObj(step)
			return completeReducingFn(rf, res)
		}
		res = step
		s = s.Rest()
	}
	return completeReducingFn(rf, res)
}

func makeMapTransducer(f Callable) Object {
	return Proc{Name: "procMapTransducerBuilder", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		rf := EnsureArgIsCallable(args, 0)
		return Proc{Name: "procMapTransducerRF", Fn: func(callArgs []Object) Object {
			switch len(callArgs) {
			case 0:
				return rf.Call(nil)
			case 1:
				return rf.Call(callArgs)
			case 2:
				mapped := f.Call([]Object{callArgs[1]})
				return rf.Call([]Object{callArgs[0], mapped})
			default:
				PanicArityMinMax(len(callArgs), 0, 2)
				return NIL
			}
		}}
	}}
}

func makeFilterTransducer(pred Callable) Object {
	return Proc{Name: "procFilterTransducerBuilder", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		rf := EnsureArgIsCallable(args, 0)
		return Proc{Name: "procFilterTransducerRF", Fn: func(callArgs []Object) Object {
			switch len(callArgs) {
			case 0:
				return rf.Call(nil)
			case 1:
				return rf.Call(callArgs)
			case 2:
				if ToBool(pred.Call([]Object{callArgs[1]})) {
					return rf.Call(callArgs)
				}
				return callArgs[0]
			default:
				PanicArityMinMax(len(callArgs), 0, 2)
				return NIL
			}
		}}
	}}
}

func makeTakeTransducer(n int) Object {
	if n < 0 {
		n = 0
	}
	remaining := n
	return Proc{Name: "procTakeTransducerBuilder", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		rf := EnsureArgIsCallable(args, 0)
		return Proc{Name: "procTakeTransducerRF", Fn: func(callArgs []Object) Object {
			switch len(callArgs) {
			case 0:
				return rf.Call(nil)
			case 1:
				return rf.Call(callArgs)
			case 2:
				if remaining <= 0 {
					return ensureReducedObj(callArgs[0])
				}
				out := rf.Call(callArgs)
				remaining--
				if remaining <= 0 {
					return ensureReducedObj(out)
				}
				return out
			default:
				PanicArityMinMax(len(callArgs), 0, 2)
				return NIL
			}
		}}
	}}
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

	// reduced family
	reducedVr := ns.Intern(MakeSymbol("reduced"))
	reducedVr.Value = Proc{Name: "procReducedCompat", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		return reducedWrap(args[0])
	}}
	referToUser(MakeSymbol("reduced"), reducedVr)

	reducedQVr := ns.Intern(MakeSymbol("reduced?"))
	reducedQVr.Value = Proc{Name: "procReducedQCompat", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		return MakeBoolean(isReducedObj(args[0]))
	}}
	referToUser(MakeSymbol("reduced?"), reducedQVr)

	ensureReducedVr := ns.Intern(MakeSymbol("ensure-reduced"))
	ensureReducedVr.Value = Proc{Name: "procEnsureReducedCompat", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		return ensureReducedObj(args[0])
	}}
	referToUser(MakeSymbol("ensure-reduced"), ensureReducedVr)

	unreducedVr := ns.Intern(MakeSymbol("unreduced"))
	unreducedVr.Value = Proc{Name: "procUnreducedCompat", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		return unreducedObj(args[0])
	}}
	referToUser(MakeSymbol("unreduced"), unreducedVr)

	// completing
	completingVr := ns.Intern(MakeSymbol("completing"))
	completingVr.Value = Proc{Name: "procCompletingCompat", Fn: func(args []Object) Object {
		if len(args) != 1 && len(args) != 2 {
			PanicArityMinMax(len(args), 1, 2)
		}
		f := EnsureArgIsCallable(args, 0)
		var cf Callable
		if len(args) == 2 {
			cf = EnsureArgIsCallable(args, 1)
		} else {
			cf = Proc{Name: "procCompletingIdentity", Fn: func(callArgs []Object) Object {
				CheckArity(callArgs, 1, 1)
				return callArgs[0]
			}}
		}
		return Proc{Name: "procCompletingRF", Fn: func(callArgs []Object) Object {
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
	if mapVr == nil || filterVr == nil || takeVr == nil || sequenceVr == nil {
		return
	}

	mapOrig, mapOK := mapVr.Value.(Callable)
	filterOrig, filterOK := filterVr.Value.(Callable)
	takeOrig, takeOK := takeVr.Value.(Callable)
	sequenceOrig, sequenceOK := sequenceVr.Value.(Callable)
	if !mapOK || !filterOK || !takeOK || !sequenceOK {
		return
	}

	// map transducer arity
	mapVr.Value = Proc{Name: "procMapTransducerCompat", Fn: func(args []Object) Object {
		if len(args) == 1 {
			f := EnsureArgIsCallable(args, 0)
			return makeMapTransducer(f)
		}
		return mapOrig.Call(args)
	}}

	// filter transducer arity
	filterVr.Value = Proc{Name: "procFilterTransducerCompat", Fn: func(args []Object) Object {
		if len(args) == 1 {
			pred := EnsureArgIsCallable(args, 0)
			return makeFilterTransducer(pred)
		}
		return filterOrig.Call(args)
	}}

	// take transducer arity
	takeVr.Value = Proc{Name: "procTakeTransducerCompat", Fn: func(args []Object) Object {
		if len(args) == 1 {
			n := EnsureArgIsNumber(args, 0).Int().I
			return makeTakeTransducer(n)
		}
		return takeOrig.Call(args)
	}}

	transduceProc := Proc{Name: "procTransduceCompat", Fn: func(args []Object) Object {
		if len(args) != 3 && len(args) != 4 {
			PanicArityMinMax(len(args), 3, 4)
		}

		xform := EnsureArgIsCallable(args, 0)
		reducingFnObj := args[1]
		f := EnsureArgIsCallable(args, 1)

		var init Object
		var collObj Object
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

	eductionVr := ns.Intern(MakeSymbol("eduction"))
	eductionVr.Value = Proc{Name: "procEductionCompat", Fn: func(args []Object) Object {
		if len(args) < 2 {
			PanicArityMinMax(len(args), 2, 999)
		}
		collObj := args[len(args)-1]
		var xformObj Object
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

		rf := Proc{Name: "procEductionConjRF", Fn: func(callArgs []Object) Object {
			switch len(callArgs) {
			case 0:
				return EmptyArrayVector()
			case 1:
				return callArgs[0]
			case 2:
				acc, ok := callArgs[0].(Conjable)
				if !ok {
					panic(FailArg(callArgs[0], "Conjable", 0))
				}
				return acc.Conj(callArgs[1]).(Object)
			default:
				PanicArityMinMax(len(callArgs), 0, 2)
				return NIL
			}
		}}

		return transduceInternal(xform, rf, EmptyArrayVector(), collObj)
	}}
	referToUser(MakeSymbol("eduction"), eductionVr)

	// sequence 2-arity compatibility: (sequence xform coll)
	sequenceVr.Value = Proc{Name: "procSequenceCompat", Fn: func(args []Object) Object {
		if len(args) == 2 {
			res := eductionVr.Value.(Callable).Call(args)
			if s, ok := res.(Seqable); ok {
				return s.Seq()
			}
			return NIL
		}
		return sequenceOrig.Call(args)
	}}
}

func init() {
	installTransducerCompat()
}
