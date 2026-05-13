package core

import corestr "github.com/rcarmo/go-joker/core/string"

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
	res := EmptyArrayVector()
	for _, token := range corestr.SplitWhitespace(s) {
		res.Append(String{S: token})
	}
	return res
}

var procFrequencies ProcFn = func(args []Object) Object {
	CheckArity(args, 1, 1)
	seq := EnsureObjectIsSeqable(args[0], "frequencies requires a seqable collection").Seq()
	if seq.IsEmpty() {
		return EmptyArrayMap()
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
			if s, ok := obj.(String); ok {
				stringCounts[s.S]++
				seq = seq.Rest()
				continue
			}
			stringOnly = false
			tm = MapToTransient(nil)
			for k, v := range stringCounts {
				tm.AssocInPlace(String{S: k}, Int{I: v})
			}
			stringCounts = nil
		}
		_, old := tm.Get(obj)
		cnt := 0
		if i, ok := old.(Int); ok {
			cnt = i.I
		}
		tm.AssocInPlace(obj, Int{I: cnt + 1})
		seq = seq.Rest()
	}
	if stringOnly {
		if len(stringCounts) <= int(HASHMAP_THRESHOLD/2) {
			res := EmptyArrayMap()
			for k, v := range stringCounts {
				res.Add(String{S: k}, Int{I: v})
			}
			return res
		}
		res := EmptyHashMap
		for k, v := range stringCounts {
			res = res.Assoc(String{S: k}, Int{I: v}).(*HashMap)
		}
		return res
	}
	return tm.ToPersistent()
}
