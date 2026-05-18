package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"sync"
)

type TransientVector = coretypes.TransientVector
type TransientMap = coretypes.TransientMap

func ToTransient(v *ArrayVector) *TransientVector {
	return coretypes.ToTransient(v.arr)
}

func MapToTransient(m coretypes.Map) *TransientMap {
	return coretypes.MapToTransient(m)
}

var transientProcsOnce sync.Once

func init() {
	installTransientBridges()
	initTransientProcs()
}

func installTransientBridges() {
	if coretypes.TransientMutationError == nil {
		coretypes.TransientMutationError = func() any { return RT.NewError("Cannot mutate a frozen transient") }
	}
	if coretypes.TransientVectorIndexTypeError == nil {
		coretypes.TransientVectorIndexTypeError = func(obj coretypes.Object) any { return RT.NewArgTypeError(1, obj, "Int") }
	}
	if coretypes.TransientVectorToPersistent == nil {
		coretypes.TransientVectorToPersistent = func(arr []coretypes.Object) coretypes.Object { return &ArrayVector{arr: arr} }
	}
	if coretypes.TransientMapToPersistent == nil {
		coretypes.TransientMapToPersistent = func(tm *coretypes.TransientMap) coretypes.Object {
			if tm.CountN <= int(HASHMAP_THRESHOLD/2) {
				res := collectionConstruction.NewEmptyArrayMap()
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
			res := EmptyHashMap
			for k, v := range tm.SM {
				res = res.Assoc(coretypes.String{S: k}, v).(*HashMap)
			}
			for _, bucket := range tm.M {
				for _, e := range bucket {
					res = res.Assoc(e.Key, e.Val).(*HashMap)
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
	CheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *ArrayVector:
		return ToTransient(coll)
	case coretypes.Map:
		return MapToTransient(coll)
	default:
		panic(RT.NewError("transient not supported on: " + coll.GetType().ToString(false)))
	}
}

var procAssocBang = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 3, 3)
	switch coll := args[0].(type) {
	case *TransientVector:
		return coll.AssocInPlace(args[1], args[2])
	case *TransientMap:
		return coll.AssocInPlace(args[1], args[2])
	default:
		panic(RT.NewError("assoc! requires a transient, got: " + coll.GetType().ToString(false)))
	}
}

var procConjBang = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 2)
	switch coll := args[0].(type) {
	case *TransientVector:
		return coll.ConjInPlace(args[1])
	case *TransientMap:
		if seq, ok := args[1].(coretypes.Seqable); ok {
			s := seq.Seq()
			if !s.IsEmpty() {
				k := s.First()
				s = s.Rest()
				if !s.IsEmpty() {
					return coll.AssocInPlace(k, s.First())
				}
			}
		}
		panic(RT.NewError("conj! on transient map requires a key/value pair"))
	default:
		panic(RT.NewError("conj! requires a transient, got: " + coll.GetType().ToString(false)))
	}
}

var procPersistentBang = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *TransientVector:
		return coll.ToPersistent()
	case *TransientMap:
		return coll.ToPersistent()
	default:
		panic(RT.NewError("persistent! requires a transient, got: " + coll.GetType().ToString(false)))
	}
}

var procIsTransient = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	switch args[0].(type) {
	case *TransientVector, *TransientMap:
		return coretypes.MakeBoolean(true)
	default:
		return coretypes.MakeBoolean(false)
	}
}

var procPopBang = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *TransientVector:
		return coll.PopInPlace()
	default:
		panic(RT.NewError("pop! requires a transient vector, got: " + coll.GetType().ToString(false)))
	}
}
