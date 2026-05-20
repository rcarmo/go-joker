package core

// sorted_colls.go — sorted-map, sorted-set, sorted-map-by, sorted-set-by.
//
// Implementation: delegates to corecollections.ArrayMap/corecollections.MapSet but sorts entries on creation.
// Not a true balanced tree — O(n log n) creation, O(n) lookup.
// Sufficient for parity; can be upgraded to a tree later.

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

var sortedMetaCache coretypes.Map

func sortedCollMeta() coretypes.Map {
	if sortedMetaCache != nil {
		return sortedMetaCache
	}
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "sorted"), coretypes.Boolean{B: true})
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
	smVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sorted-map"))
	smVr.Value = Proc{Name: "procSortedMap", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args)%2 != 0 {
			panic(coretypes.RuntimeError("sorted-map requires an even number of arguments"))
		}
		pairs := sortedKeyValuePairs(args, nil)
		m := corecollections.EmptyArrayMap()
		for _, p := range pairs {
			addOrReplaceSortedMap(m, p.Key, p.Val, nil)
		}
		return m.WithMeta(sortedCollMeta())
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "sorted-map"), smVr)

	// sorted-map-by — (sorted-map-by comparator k1 v1 k2 v2 ...)
	smbVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sorted-map-by"))
	smbVr.Value = Proc{Name: "procSortedMapBy", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 999)
		comp := coretypes.EnsureArgIsCallable(args, 0)
		keyvals := args[1:]
		if len(keyvals)%2 != 0 {
			panic(coretypes.RuntimeError("sorted-map-by requires an even number of key/value arguments"))
		}
		pairs := sortedKeyValuePairs(keyvals, comp)
		m := corecollections.EmptyArrayMap()
		for _, p := range pairs {
			addOrReplaceSortedMap(m, p.Key, p.Val, comp)
		}
		return m.WithMeta(sortedCollMeta())
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "sorted-map-by"), smbVr)

	// sorted-set — (sorted-set v1 v2 ...)
	ssVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sorted-set"))
	ssVr.Value = Proc{Name: "procSortedSet", Fn: func(args []coretypes.Object) coretypes.Object {
		return sortedSetFrom(args, nil)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "sorted-set"), ssVr)

	// sorted-set-by — (sorted-set-by comparator v1 v2 ...)
	ssbVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sorted-set-by"))
	ssbVr.Value = Proc{Name: "procSortedSetBy", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 999)
		return sortedSetFrom(args[1:], coretypes.EnsureArgIsCallable(args, 0))
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "sorted-set-by"), ssbVr)

	// sorted? — (sorted? coll)
	sortedQVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sorted?"))
	sortedQVr.Value = Proc{Name: "procSortedQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		if m, ok := args[0].(coretypes.Meta); ok {
			meta := m.GetMeta()
			if meta != nil {
				if ok, v := meta.Get(coretypes.MakeKeyword(STRINGS.Intern, "sorted")); ok {
					return coretypes.MakeBoolean(ToBool(v))
				}
			}
		}
		return coretypes.Boolean{B: false}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "sorted?"), sortedQVr)

	// subseq/rsubseq — range queries over sorted coll API.
	subseqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "subseq"))
	subseqVr.Value = Proc{Name: "procSubseq", Fn: func(args []coretypes.Object) coretypes.Object {
		return sortedSubseq(args, false)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "subseq"), subseqVr)

	rsubseqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "rsubseq"))
	rsubseqVr.Value = Proc{Name: "procRsubseq", Fn: func(args []coretypes.Object) coretypes.Object {
		return sortedSubseq(args, true)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "rsubseq"), rsubseqVr)

	// comparator — (comparator pred) — wraps a boolean predicate into a comparator fn
	compVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "comparator"))
	compVr.Value = Proc{Name: "procComparator", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		pred := coretypes.EnsureArgIsCallable(args, 0)
		return Proc{Name: "procComparatorFn", Fn: func(cArgs []coretypes.Object) coretypes.Object {
			runtimeCheckArity(cArgs, 2, 2)
			if ToBool(pred.Call(cArgs)) {
				return coretypes.Int{I: -1}
			}
			if ToBool(call2(pred, cArgs[1], cArgs[0])) {
				return coretypes.Int{I: 1}
			}
			return coretypes.Int{I: 0}
		}}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "comparator"), compVr)
}

func sortedKeyValuePairs(keyvals []coretypes.Object, comp coretypes.Callable) []corecollections.KV[coretypes.Object, coretypes.Object] {
	pairs := corecollections.FlatToKVs(keyvals)
	corecollections.SortKVsBy(pairs, func(a, b coretypes.Object) bool {
		if comp != nil {
			return compareWith(comp, a, b) < 0
		}
		return compareObjects(a, b) < 0
	})
	return pairs
}

func addOrReplaceSortedMap(m *corecollections.ArrayMap, key coretypes.Object, val coretypes.Object, comp coretypes.Callable) {
	if comp != nil {
		for i := 0; i < len(m.Arr); i += 2 {
			if compareWith(comp, m.Arr[i], key) == 0 {
				m.Arr[i] = key
				m.Arr[i+1] = val
				return
			}
		}
		m.Add(key, val)
		return
	}
	if m.Add(key, val) {
		return
	}
	if i := corecollections.MapIndexOf(m.Arr, key); i != -1 {
		m.Arr[i+1] = val
	}
}

func sortedSetFrom(values []coretypes.Object, comp coretypes.Callable) coretypes.Object {
	sorted := make([]coretypes.Object, len(values))
	copy(sorted, values)
	corecollections.SortBy(sorted, func(a, b coretypes.Object) bool {
		if comp != nil {
			return compareWith(comp, a, b) < 0
		}
		return compareObjects(a, b) < 0
	})
	s := corecollections.EmptySet()
	for _, v := range sorted {
		s = s.Conj(v).(*corecollections.MapSet)
	}
	return s.WithMeta(sortedCollMeta())
}

func compareWith(comp coretypes.Callable, a, b coretypes.Object) int {
	return compare(comp, a, b)
}

func sortedSubseq(args []coretypes.Object, reverse bool) coretypes.Object {
	if len(args) != 3 && len(args) != 5 {
		coretypes.RuntimePanicArityMinMax(len(args), 3, 5)
	}
	coll := args[0]
	entries := sortedEntries(coll)
	if reverse {
		corecollections.Reverse(entries)
	}
	startPred := coretypes.EnsureObjectIsCallable(args[1], "subseq predicate must be callable, got %s")
	startKey := args[2]
	var endPred coretypes.Callable
	var endKey coretypes.Object
	if len(args) == 5 {
		endPred = coretypes.EnsureObjectIsCallable(args[3], "subseq predicate must be callable, got %s")
		endKey = args[4]
	}
	out := make([]coretypes.Object, 0)
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
	return &corecollections.ArraySeq{Arr: out, Index: 0}
}

func sortedEntries(coll coretypes.Object) []coretypes.Object {
	out := make([]coretypes.Object, 0)
	preserveOrder := isSortedColl(coll)
	if m, ok := coll.(coretypes.Map); ok {
		for it := m.Iter(); it.HasNext(); {
			p := it.Next()
			out = append(out, corecollections.NewArrayVectorFrom(p.Key, p.Value))
		}
		if !preserveOrder {
			corecollections.SortBy(out, func(a, b coretypes.Object) bool { return compareObjects(rangeKey(a), rangeKey(b)) < 0 })
		}
		return out
	}
	if s, ok := coll.(coretypes.Seqable); ok {
		for seq := s.Seq(); !seq.IsEmpty(); seq = seq.Rest() {
			out = append(out, seq.First())
		}
		if !preserveOrder {
			corecollections.SortBy(out, func(a, b coretypes.Object) bool { return compareObjects(a, b) < 0 })
		}
	}
	return out
}

func isSortedColl(coll coretypes.Object) bool {
	m, ok := coll.(coretypes.Meta)
	if !ok || m.GetMeta() == nil {
		return false
	}
	ok, v := m.GetMeta().Get(coretypes.MakeKeyword(STRINGS.Intern, "sorted"))
	return ok && ToBool(v)
}

func rangeKey(entry coretypes.Object) coretypes.Object {
	if v, ok := entry.(coretypes.Vec); ok && v.Count() >= 1 {
		return v.Nth(0)
	}
	return entry
}

func rangePred(pred coretypes.Callable, a, b coretypes.Object) bool {
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
func compareObjects(a, b coretypes.Object) int {
	return corecollections.CompareObjectsDefault(a, b)
}
