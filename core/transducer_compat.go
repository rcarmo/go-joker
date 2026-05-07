package core

// transducer_compat.go — Transducer runtime support with proper Reduced type.
//
// Provides full Clojure transducer semantics:
// - transducer arities for map/filter/take
// - transduce (3-arity and 4-arity)
// - reduced, reduced?, ensure-reduced, unreduced
// - completing (1 and 2-arity)
// - eduction (materialized vector-backed)
// - sequence 2-arity via eduction

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
	return DerefReduced(completed)
}

func transduceInternal(xform Callable, reducingFnObj Object, init Object, collObj Object) Object {
	rfObj := xform.Call([]Object{reducingFnObj})
	rf := EnsureObjectIsCallable(rfObj, "transduce xform must produce a reducing function, got %s")

	s := EnsureObjectIsSeqable(collObj, "Arg of core/transduce must be Seqable, got %s").Seq()
	res := init
	for !s.IsEmpty() {
		step := rf.Call([]Object{res, s.First()})
		if IsReduced(step) {
			res = DerefReduced(step)
			return completeReducingFn(rf, res)
		}
		res = step
		s = s.Rest()
	}
	return completeReducingFn(rf, res)
}

func makeMapTransducer(f Callable) Object {
	return Proc{Name: "procMapXf", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		rf := EnsureArgIsCallable(args, 0)
		return Proc{Name: "procMapXfRF", Fn: func(callArgs []Object) Object {
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
	return Proc{Name: "procFilterXf", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		rf := EnsureArgIsCallable(args, 0)
		return Proc{Name: "procFilterXfRF", Fn: func(callArgs []Object) Object {
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
	limit := n
	return Proc{Name: "procTakeXf", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		rf := EnsureArgIsCallable(args, 0)
		remaining := limit // fresh counter per builder invocation
		return Proc{Name: "procTakeXfRF", Fn: func(callArgs []Object) Object {
			switch len(callArgs) {
			case 0:
				return rf.Call(nil)
			case 1:
				return rf.Call(callArgs)
			case 2:
				if remaining <= 0 {
					return EnsureReduced(callArgs[0])
				}
				out := rf.Call(callArgs)
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

	// Fix reduce-kv to handle nil coll (returns init)
	rkvVr := ns.Resolve("reduce-kv")
	if rkvVr != nil {
		origRKV, ok := rkvVr.Value.(Callable)
		if ok {
			rkvVr.Value = Proc{Name: "procReduceKvNilSafe", Fn: func(args []Object) Object {
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
	reducedVr.Value = Proc{Name: "procReduced", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		return MakeReduced(args[0])
	}}
	referToUser(MakeSymbol("reduced"), reducedVr)

	// reduced? — type check, no map lookup
	reducedQVr := ns.Intern(MakeSymbol("reduced?"))
	reducedQVr.Value = Proc{Name: "procReducedQ", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		return MakeBoolean(IsReduced(args[0]))
	}}
	referToUser(MakeSymbol("reduced?"), reducedQVr)

	// ensure-reduced
	ensureReducedVr := ns.Intern(MakeSymbol("ensure-reduced"))
	ensureReducedVr.Value = Proc{Name: "procEnsureReduced", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		return EnsureReduced(args[0])
	}}
	referToUser(MakeSymbol("ensure-reduced"), ensureReducedVr)

	// unreduced — deref a Reduced box (identity if not reduced)
	unreducedVr := ns.Intern(MakeSymbol("unreduced"))
	unreducedVr.Value = Proc{Name: "procUnreduced", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		return DerefReduced(args[0])
	}}
	referToUser(MakeSymbol("unreduced"), unreducedVr)

	// completing — wraps a reducing fn with optional completion step
	completingVr := ns.Intern(MakeSymbol("completing"))
	completingVr.Value = Proc{Name: "procCompleting", Fn: func(args []Object) Object {
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

	// map transducer arity: (map f) returns a transducer
	mapVr.Value = Proc{Name: "procMapXfCompat", Fn: func(args []Object) Object {
		if len(args) == 1 {
			f := EnsureArgIsCallable(args, 0)
			return makeMapTransducer(f)
		}
		return mapOrig.Call(args)
	}}

	// filter transducer arity: (filter pred) returns a transducer
	filterVr.Value = Proc{Name: "procFilterXfCompat", Fn: func(args []Object) Object {
		if len(args) == 1 {
			pred := EnsureArgIsCallable(args, 0)
			return makeFilterTransducer(pred)
		}
		return filterOrig.Call(args)
	}}

	// take transducer arity: (take n) returns a transducer when used with transduce
	takeVr.Value = Proc{Name: "procTakeXfCompat", Fn: func(args []Object) Object {
		if len(args) == 1 {
			n := EnsureArgIsNumber(args, 0).Int().I
			return makeTakeTransducer(n)
		}
		return takeOrig.Call(args)
	}}

	// transduce — full 3 and 4-arity support
	transduceProc := Proc{Name: "procTransduce", Fn: func(args []Object) Object {
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

	// eduction — materializes transducer pipeline into a vector
	eductionVr := ns.Intern(MakeSymbol("eduction"))
	eductionVr.Value = Proc{Name: "procEduction", Fn: func(args []Object) Object {
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

		conjRF := Proc{Name: "procEductionConjRF", Fn: func(callArgs []Object) Object {
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

		return transduceInternal(xform, conjRF, EmptyArrayVector(), collObj)
	}}
	referToUser(MakeSymbol("eduction"), eductionVr)

	// sequence 2-arity: (sequence xform coll) → lazy seq of eduction result
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
	maybeOverrideRange()
}
