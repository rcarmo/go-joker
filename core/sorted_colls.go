package core

// sorted_colls.go — sorted-map, sorted-set, sorted-map-by, sorted-set-by.
//
// Implementation: delegates to ArrayMap/MapSet but sorts entries on creation.
// Not a true balanced tree — O(n log n) creation, O(n) lookup.
// Sufficient for parity; can be upgraded to a tree later.

import "sort"

var sortedMetaCache Map

func sortedCollMeta() Map {
	if sortedMetaCache != nil {
		return sortedMetaCache
	}
	m := EmptyArrayMap()
	m.Add(MakeKeyword("sorted"), Boolean{B: true})
	sortedMetaCache = m
	return sortedMetaCache
}

func init() {
	registerSortedCollProcs()
}

func registerSortedCollProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// sorted-map — (sorted-map k1 v1 k2 v2 ...)
	smVr := ns.Intern(MakeSymbol("sorted-map"))
	smVr.Value = Proc{Name: "procSortedMap", Fn: func(args []Object) Object {
		if len(args)%2 != 0 {
			panic(RT.NewError("sorted-map requires an even number of arguments"))
		}
		// Collect key-value pairs
		type kv struct {
			key Object
			val Object
		}
		pairs := make([]kv, len(args)/2)
		for i := 0; i < len(args); i += 2 {
			pairs[i/2] = kv{args[i], args[i+1]}
		}
		// Sort by key
		sort.Slice(pairs, func(i, j int) bool {
			return compareObjects(pairs[i].key, pairs[j].key) < 0
		})
		m := EmptyArrayMap()
		for _, p := range pairs {
			m.Add(p.key, p.val)
		}
		return m.WithMeta(sortedCollMeta())
	}}
	referToUser(MakeSymbol("sorted-map"), smVr)

	// sorted-set — (sorted-set v1 v2 ...)
	ssVr := ns.Intern(MakeSymbol("sorted-set"))
	ssVr.Value = Proc{Name: "procSortedSet", Fn: func(args []Object) Object {
		sorted := make([]Object, len(args))
		copy(sorted, args)
		sort.Slice(sorted, func(i, j int) bool {
			return compareObjects(sorted[i], sorted[j]) < 0
		})
		s := EmptySet()
		for _, v := range sorted {
			s = s.Conj(v).(*MapSet)
		}
		return s.WithMeta(sortedCollMeta())
	}}
	referToUser(MakeSymbol("sorted-set"), ssVr)

	// sorted? — (sorted? coll)
	sortedQVr := ns.Intern(MakeSymbol("sorted?"))
	sortedQVr.Value = Proc{Name: "procSortedQ", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		if m, ok := args[0].(Meta); ok {
			meta := m.GetMeta()
			if meta != nil {
				if ok, v := meta.Get(MakeKeyword("sorted")); ok {
					return MakeBoolean(ToBool(v))
				}
			}
		}
		return Boolean{B: false}
	}}
	referToUser(MakeSymbol("sorted?"), sortedQVr)

	// comparator — (comparator pred) — wraps a boolean predicate into a comparator fn
	compVr := ns.Intern(MakeSymbol("comparator"))
	compVr.Value = Proc{Name: "procComparator", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		pred := EnsureArgIsCallable(args, 0)
		return Proc{Name: "procComparatorFn", Fn: func(cArgs []Object) Object {
			CheckArity(cArgs, 2, 2)
			if ToBool(pred.Call(cArgs)) {
				return Int{I: -1}
			}
			if ToBool(call2(pred, cArgs[1], cArgs[0])) {
				return Int{I: 1}
			}
			return Int{I: 0}
		}}
	}}
	referToUser(MakeSymbol("comparator"), compVr)
}

// compareObjects provides a default ordering for Clojure values.
func compareObjects(a, b Object) int {
	// Same type comparisons
	switch av := a.(type) {
	case Int:
		if bv, ok := b.(Int); ok {
			if av.I < bv.I {
				return -1
			}
			if av.I > bv.I {
				return 1
			}
			return 0
		}
	case Double:
		if bv, ok := b.(Double); ok {
			if av.D < bv.D {
				return -1
			}
			if av.D > bv.D {
				return 1
			}
			return 0
		}
	case String:
		if bv, ok := b.(String); ok {
			if av.S < bv.S {
				return -1
			}
			if av.S > bv.S {
				return 1
			}
			return 0
		}
	case Keyword:
		if bv, ok := b.(Keyword); ok {
			as := av.ToString(false)
			bs := bv.ToString(false)
			if as < bs {
				return -1
			}
			if as > bs {
				return 1
			}
			return 0
		}
	case Symbol:
		if bv, ok := b.(Symbol); ok {
			as := av.ToString(false)
			bs := bv.ToString(false)
			if as < bs {
				return -1
			}
			if as > bs {
				return 1
			}
			return 0
		}
	}
	// Fall back to string comparison
	as := a.ToString(false)
	bs := b.ToString(false)
	if as < bs {
		return -1
	}
	if as > bs {
		return 1
	}
	return 0
}
