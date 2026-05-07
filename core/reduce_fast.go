package core

// reduce_fast.go — Fast-path reduce for seqable types that don't implement the Reduce interface.
// This closes the 42x gap on (reduce + 0 (range 1000000)) by walking the seq directly
// instead of requiring the Reduce interface.

// seqReduceInit walks a Seq and applies f(acc, elem) for each element.
// Supports early termination via Reduced.
func seqReduceInit(s Seq, f Callable, init Object) Object {
	acc := init
	for !s.IsEmpty() {
		acc = f.Call([]Object{acc, s.First()})
		if isReducedObj(acc) {
			return unreducedObj(acc)
		}
		s = s.Rest()
	}
	return acc
}

// seqReduce walks a Seq using (f (first s) (second s)) as initial, then continues.
func seqReduce(s Seq, f Callable) Object {
	if s.IsEmpty() {
		return f.Call(nil)
	}
	acc := s.First()
	s = s.Rest()
	for !s.IsEmpty() {
		acc = f.Call([]Object{acc, s.First()})
		if isReducedObj(acc) {
			return unreducedObj(acc)
		}
		s = s.Rest()
	}
	return acc
}

func init() {
	// Override procReduce to support Seqable types that don't implement Reduce.
	// This makes (reduce + 0 (range 1000000)) work without requiring LazySeq
	// to implement the Reduce interface.
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	reduceVr := ns.Resolve("reduce__")
	if reduceVr == nil {
		return
	}

	reduceVr.Value = Proc{Name: "procReduceFast", Fn: func(args []Object) Object {
		f := EnsureArgIsCallable(args, 0)
		if len(args) == 2 {
			obj := args[1]
			if coll, ok := obj.(Reduce); ok {
				return coll.reduce(f)
			}
			if s, ok := obj.(Seqable); ok {
				return seqReduce(s.Seq(), f)
			}
			panic(FailArg(obj, "Reduce or Seqable", 1))
		}
		init := args[1]
		obj := args[2]
		if coll, ok := obj.(Reduce); ok {
			return coll.reduceInit(f, init)
		}
		if s, ok := obj.(Seqable); ok {
			return seqReduceInit(s.Seq(), f, init)
		}
		panic(FailArg(obj, "Reduce or Seqable", 2))
	}}

	// Also update the public-facing reduce var if it exists
	pubVr := ns.Resolve("reduce")
	if pubVr != nil && pubVr != reduceVr {
		origReduce, ok := pubVr.Value.(Callable)
		if ok {
			pubVr.Value = Proc{Name: "procReducePublicFast", Fn: func(args []Object) Object {
				// Try the fast path first via the internal reduce__
				func() {
					defer func() {
						recover() // swallow; fall back to original
					}()
				}()
				// Attempt direct fast-path
				f := EnsureArgIsCallable(args, 0)
				if len(args) == 2 {
					obj := args[1]
					if coll, ok := obj.(Reduce); ok {
						return coll.reduce(f)
					}
					if s, ok := obj.(Seqable); ok {
						return seqReduce(s.Seq(), f)
					}
				} else if len(args) == 3 {
					init := args[1]
					obj := args[2]
					if coll, ok := obj.(Reduce); ok {
						return coll.reduceInit(f, init)
					}
					if s, ok := obj.(Seqable); ok {
						return seqReduceInit(s.Seq(), f, init)
					}
				}
				return origReduce.Call(args)
			}}
		}
	}
}
