package core

import (
	"sync"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

var transientProcsOnce sync.Once

func init() {
	installTransientBridges()
	initTransientProcs()
}

func installTransientBridges() {
	if coretypes.TransientMutationError == nil {
		coretypes.TransientMutationError = func() any { return coretypes.RuntimeError("Cannot mutate a frozen transient") }
	}
	if coretypes.TransientVectorIndexTypeError == nil {
		coretypes.TransientVectorIndexTypeError = func(obj coretypes.Object) any { return RT.NewArgTypeError(1, obj, "Int") }
	}
	if coretypes.TransientVectorToPersistent == nil {
		coretypes.TransientVectorToPersistent = func(arr []coretypes.Object) coretypes.Object { return &corecollections.ArrayVector{Arr: arr} }
	}
	if coretypes.TransientMapToPersistent == nil {
		coretypes.TransientMapToPersistent = func(tm *coretypes.TransientMap) coretypes.Object {
			if tm.CountN <= int(corecollections.HASHMAP_THRESHOLD/2) {
				res := corecollections.EmptyArrayMap()
				for k, v := range tm.SM {
					res.Add(coretypes.String{S: k}, v)
				}
				for _, bucket := range tm.M {
					for _, e := range bucket {
						res.Add(e.Key, e.Val)
					}
				}
				return res
			}
			res := corecollections.EmptyHashMap
			for k, v := range tm.SM {
				res = res.Assoc(coretypes.String{S: k}, v).(*corecollections.HashMap)
			}
			for _, bucket := range tm.M {
				for _, e := range bucket {
					res = res.Assoc(e.Key, e.Val).(*corecollections.HashMap)
				}
			}
			return res
		}
	}
}

func initTransientProcs() {
	transientProcsOnce.Do(func() {
		ns := GLOBAL_ENV.CoreNamespace
		procs := []struct {
			name  string
			fn    func([]coretypes.Object) coretypes.Object
			pname string
		}{
			{"transient", procTransient, "procTransient"},
			{"assoc!", procAssocBang, "procAssocBang"},
			{"conj!", procConjBang, "procConjBang"},
			{"persistent!", procPersistentBang, "procPersistentBang"},
		}
		for _, p := range procs {
			sym := coretypes.MakeSymbol(STRINGS.Intern, p.name)
			vr := ns.Intern(sym)
			vr.Value = Proc{Fn: p.fn, Name: p.pname}
			referToUser(sym, vr)
		}

		tqSym := coretypes.MakeSymbol(STRINGS.Intern, "transient?")
		tqVr := ns.Intern(tqSym)
		tqVr.Value = Proc{Name: "procTransientQ", Fn: procIsTransient}
		referToUser(tqSym, tqVr)

		popSym := coretypes.MakeSymbol(STRINGS.Intern, "pop!")
		popVr := ns.Intern(popSym)
		popVr.Value = Proc{Name: "procPopBang", Fn: procPopBang}
		referToUser(popSym, popVr)
	})
}

var procTransient = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *corecollections.ArrayVector:
		return coretypes.ToTransient(coll.Arr)
	case coretypes.Map:
		return coretypes.MapToTransient(coll)
	default:
		panic(coretypes.RuntimeError("transient not supported on: " + coll.GetType().ToString(false)))
	}
}

var procAssocBang = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 3, 3)
	switch coll := args[0].(type) {
	case *coretypes.TransientVector:
		return coll.AssocInPlace(args[1], args[2])
	case *coretypes.TransientMap:
		return coll.AssocInPlace(args[1], args[2])
	default:
		panic(coretypes.RuntimeError("assoc! requires a transient, got: " + coll.GetType().ToString(false)))
	}
}

var procConjBang = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 2, 3)
	switch coll := args[0].(type) {
	case *coretypes.TransientVector:
		if len(args) != 2 {
			coretypes.RuntimePanicArityMinMax(len(args), 2, 2)
		}
		return coll.ConjInPlace(args[1])
	case *coretypes.TransientMap:
		if len(args) == 3 {
			return coll.AssocInPlace(args[1], args[2])
		}
		if k, v, ok := corecollections.TransientMapConjEntry(args[1]); ok {
			return coll.AssocInPlace(k, v)
		}
		panic(coretypes.RuntimeError("conj! on transient map requires a key/value pair"))
	default:
		panic(coretypes.RuntimeError("conj! requires a transient, got: " + coll.GetType().ToString(false)))
	}
}

var procPersistentBang = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *coretypes.TransientVector:
		return coll.ToPersistent()
	case *coretypes.TransientMap:
		return coll.ToPersistent()
	default:
		panic(coretypes.RuntimeError("persistent! requires a transient, got: " + coll.GetType().ToString(false)))
	}
}

var procIsTransient = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	return coretypes.MakeBoolean(corecollections.IsTransientObject(args[0]))
}

var procPopBang = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *coretypes.TransientVector:
		return coll.PopInPlace()
	default:
		panic(coretypes.RuntimeError("pop! requires a transient vector, got: " + coll.GetType().ToString(false)))
	}
}
