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
	m := collections.EmptyArrayMap()
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
		m := collections.EmptyArrayMap()
		for _, p := range pairs {
			m.Add(p.key, p.val)
		}
		return m.WithMeta(sortedCollMeta())
	}}
	referToUser(MakeSymbol("sorted-map"), smVr)

	// sorted-map-by — (sorted-map-by comparator k1 v1 k2 v2 ...)
	smbVr := ns.Intern(MakeSymbol("sorted-map-by"))
	smbVr.Value = Proc{Name: "procSortedMapBy", Fn: func(args []Object) Object {
		CheckArity(args, 1, 999)
		comp := EnsureArgIsCallable(args, 0)
		keyvals := args[1:]
		if len(keyvals)%2 != 0 {
			panic(RT.NewError("sorted-map-by requires an even number of key/value arguments"))
		}
		type kv struct {
			key Object
			val Object
		}
		pairs := make([]kv, len(keyvals)/2)
		for i := 0; i < len(keyvals); i += 2 {
			pairs[i/2] = kv{keyvals[i], keyvals[i+1]}
		}
		sort.Slice(pairs, func(i, j int) bool {
			return compareWith(comp, pairs[i].key, pairs[j].key) < 0
		})
		m := collections.EmptyArrayMap()
		for _, p := range pairs {
			m.Add(p.key, p.val)
		}
		return m.WithMeta(sortedCollMeta())
	}}
	referToUser(MakeSymbol("sorted-map-by"), smbVr)

	// sorted-set — (sorted-set v1 v2 ...)
	ssVr := ns.Intern(MakeSymbol("sorted-set"))
	ssVr.Value = Proc{Name: "procSortedSet", Fn: func(args []Object) Object {
		return sortedSetFrom(args, nil)
	}}
	referToUser(MakeSymbol("sorted-set"), ssVr)

	// sorted-set-by — (sorted-set-by comparator v1 v2 ...)
	ssbVr := ns.Intern(MakeSymbol("sorted-set-by"))
	ssbVr.Value = Proc{Name: "procSortedSetBy", Fn: func(args []Object) Object {
		CheckArity(args, 1, 999)
		return sortedSetFrom(args[1:], EnsureArgIsCallable(args, 0))
	}}
	referToUser(MakeSymbol("sorted-set-by"), ssbVr)

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

	// subseq/rsubseq — range queries over sorted coll API.
	subseqVr := ns.Intern(MakeSymbol("subseq"))
	subseqVr.Value = Proc{Name: "procSubseq", Fn: func(args []Object) Object {
		return sortedSubseq(args, false)
	}}
	referToUser(MakeSymbol("subseq"), subseqVr)

	rsubseqVr := ns.Intern(MakeSymbol("rsubseq"))
	rsubseqVr.Value = Proc{Name: "procRsubseq", Fn: func(args []Object) Object {
		return sortedSubseq(args, true)
	}}
	referToUser(MakeSymbol("rsubseq"), rsubseqVr)

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

func sortedSetFrom(values []Object, comp Callable) Object {
	sorted := make([]Object, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool {
		if comp != nil {
			return compareWith(comp, sorted[i], sorted[j]) < 0
		}
		return compareObjects(sorted[i], sorted[j]) < 0
	})
	s := collections.EmptySet()
	for _, v := range sorted {
		s = s.Conj(v).(*MapSet)
	}
	return s.WithMeta(sortedCollMeta())
}

func compareWith(comp Callable, a, b Object) int {
	return compare(comp, a, b)
}

func sortedSubseq(args []Object, reverse bool) Object {
	if len(args) != 3 && len(args) != 5 {
		PanicArityMinMax(len(args), 3, 5)
	}
	coll := args[0]
	entries := sortedEntries(coll)
	if reverse {
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
	}
	startPred := EnsureObjectIsCallable(args[1], "subseq predicate must be callable, got %s")
	startKey := args[2]
	var endPred Callable
	var endKey Object
	if len(args) == 5 {
		endPred = EnsureObjectIsCallable(args[3], "subseq predicate must be callable, got %s")
		endKey = args[4]
	}
	out := make([]Object, 0)
	for _, e := range entries {
		k := rangeKey(e)
		if !rangePred(startPred, k, startKey) {
			continue
		}
		if endPred != nil && !rangePred(endPred, k, endKey) {
			continue
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return NIL
	}
	return &ArraySeq{arr: out, index: 0}
}

func sortedEntries(coll Object) []Object {
	out := make([]Object, 0)
	preserveOrder := isSortedColl(coll)
	if m, ok := coll.(Map); ok {
		for it := m.Iter(); it.HasNext(); {
			p := it.Next()
			out = append(out, collections.ArrayVectorFrom(p.Key, p.Value))
		}
		if !preserveOrder {
			sort.Slice(out, func(i, j int) bool { return compareObjects(rangeKey(out[i]), rangeKey(out[j])) < 0 })
		}
		return out
	}
	if s, ok := coll.(Seqable); ok {
		for seq := s.Seq(); !seq.IsEmpty(); seq = seq.Rest() {
			out = append(out, seq.First())
		}
		if !preserveOrder {
			sort.Slice(out, func(i, j int) bool { return compareObjects(out[i], out[j]) < 0 })
		}
	}
	return out
}

func isSortedColl(coll Object) bool {
	m, ok := coll.(Meta)
	if !ok || m.GetMeta() == nil {
		return false
	}
	ok, v := m.GetMeta().Get(MakeKeyword("sorted"))
	return ok && ToBool(v)
}

func rangeKey(entry Object) Object {
	if v, ok := entry.(Vec); ok && v.Count() >= 1 {
		return v.Nth(0)
	}
	return entry
}

func rangePred(pred Callable, a, b Object) bool {
	if name := hotReducerName(pred); name != "" {
		switch name {
		case "procLt":
			return compareObjects(a, b) < 0
		case "procLte":
			return compareObjects(a, b) <= 0
		case "procGt":
			return compareObjects(a, b) > 0
		case "procGte":
			return compareObjects(a, b) >= 0
		}
	}
	return ToBool(call2(pred, a, b))
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
