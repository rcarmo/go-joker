package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

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
