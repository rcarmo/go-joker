package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"sync"
)

// transient.go — Clojure-style transient data structures.
//
// Transients provide O(1) mutable access to vectors and maps within
// a single-threaded context. They are created from persistent structures,
// mutated in place, and then frozen back to persistent form.
//
// API:
//   (transient coll)        → TransientVector or TransientMap
//   (assoc! tv idx val)     → tv (mutated in place)
//   (conj! tv val)          → tv (appended in place)
//   (pop! tv)               → tv (last element removed)
//   (persistent! tv)        → persistent vector or map
//   (transient? x)          → true if x is a transient
//
// Semantics:
//   - Single-owner: do not share transients across goroutines
//   - After persistent!, the transient is invalid (further mutation panics)
//   - Transients implement coretypes.Counted, coretypes.Indexed, and coretypes.Gettable

// ---------- TransientVector ----------

// TransientVector is a mutable vector backed by a Go slice.
type TransientVector struct {
	arr    []Object
	frozen bool
}

func (tv *TransientVector) ToString(escape bool) string           { return "#<transient-vector>" }
func (tv *TransientVector) Equals(other interface{}) bool         { return tv == other }
func (tv *TransientVector) GetInfo() *coretypes.ObjectInfo        { return nil }
func (tv *TransientVector) WithInfo(*coretypes.ObjectInfo) Object { return tv }
func (tv *TransientVector) GetType() *coretypes.Type              { return TYPE.ArrayVector }
func (tv *TransientVector) Hash() uint32                          { return 0 }
func (tv *TransientVector) Count() int                            { return len(tv.arr) }

func (tv *TransientVector) checkFrozen() {
	if tv.frozen {
		panic(RT.NewError("Cannot mutate a frozen transient"))
	}
}

// AssocInPlace sets an element by index. Returns self.
func (tv *TransientVector) AssocInPlace(key, val Object) *TransientVector {
	tv.checkFrozen()
	idxObj, ok := key.(coretypes.Int)
	if !ok {
		panic(RT.NewArgTypeError(1, key, "Int"))
	}
	idx := idxObj.I
	if idx >= 0 && idx < len(tv.arr) {
		tv.arr[idx] = val
	} else if idx == len(tv.arr) {
		tv.arr = append(tv.arr, val)
	}
	return tv
}

// ConjInPlace appends an element. Returns self.
func (tv *TransientVector) ConjInPlace(val Object) *TransientVector {
	tv.checkFrozen()
	tv.arr = append(tv.arr, val)
	return tv
}

// PopInPlace removes the last element. Returns self.
func (tv *TransientVector) PopInPlace() *TransientVector {
	tv.checkFrozen()
	if len(tv.arr) > 0 {
		tv.arr = tv.arr[:len(tv.arr)-1]
	}
	return tv
}

// At returns the element at index for coretypes.CountedIndexed compatibility.
func (tv *TransientVector) At(i int) Object { return tv.Nth(i) }

// Nth returns the element at index.
func (tv *TransientVector) Nth(i int) Object {
	if i >= 0 && i < len(tv.arr) {
		return tv.arr[i]
	}
	return NIL
}

func (tv *TransientVector) TryNth(i int, d Object) Object {
	if i >= 0 && i < len(tv.arr) {
		return tv.arr[i]
	}
	return d
}

// Get implements coretypes.Gettable for transient vectors.
func (tv *TransientVector) Get(key Object) (bool, Object) {
	if idx, ok := key.(coretypes.Int); ok {
		if idx.I >= 0 && idx.I < len(tv.arr) {
			return true, tv.arr[idx.I]
		}
	}
	return false, NIL
}

// ToPersistent freezes the transient and returns a persistent vector.
func (tv *TransientVector) ToPersistent() *ArrayVector {
	tv.frozen = true
	arr := make([]Object, len(tv.arr))
	copy(arr, tv.arr)
	return &ArrayVector{arr: arr}
}

// ToTransient creates a mutable copy from an ArrayVector.
func ToTransient(v *ArrayVector) *TransientVector {
	arr := make([]Object, len(v.arr))
	copy(arr, v.arr)
	return &TransientVector{arr: arr}
}

// ---------- TransientMap ----------

// TransientMap is a mutable map backed by a Go map.
type TransientMap struct {
	m      map[uint32][]mapEntry
	sm     map[string]Object // fast side table for String keys
	count  int
	frozen bool
}

type mapEntry struct {
	key Object
	val Object
}

func (tm *TransientMap) ToString(escape bool) string           { return "#<transient-map>" }
func (tm *TransientMap) Equals(other interface{}) bool         { return tm == other }
func (tm *TransientMap) GetInfo() *coretypes.ObjectInfo        { return nil }
func (tm *TransientMap) WithInfo(*coretypes.ObjectInfo) Object { return tm }
func (tm *TransientMap) GetType() *coretypes.Type              { return TYPE.ArrayMap }
func (tm *TransientMap) Hash() uint32                          { return 0 }
func (tm *TransientMap) Count() int                            { return tm.count }

func (tm *TransientMap) checkFrozen() {
	if tm.frozen {
		panic(RT.NewError("Cannot mutate a frozen transient"))
	}
}

// AssocInPlace sets a key-value pair. Returns self.
func (tm *TransientMap) AssocInPlace(key, val Object) *TransientMap {
	tm.checkFrozen()
	if s, ok := key.(coretypes.String); ok {
		if tm.sm == nil {
			tm.sm = make(map[string]Object)
		}
		if _, exists := tm.sm[s.S]; !exists {
			tm.count++
		}
		tm.sm[s.S] = val
		return tm
	}
	if tm.m == nil {
		tm.m = make(map[uint32][]mapEntry)
	}
	h := key.Hash()
	bucket := tm.m[h]
	for i, e := range bucket {
		if e.key.Equals(key) {
			tm.m[h][i].val = val
			return tm
		}
	}
	tm.m[h] = append(bucket, mapEntry{key, val})
	tm.count++
	return tm
}

// Get implements coretypes.Gettable for transient maps.
func (tm *TransientMap) Get(key Object) (bool, Object) {
	if s, ok := key.(coretypes.String); ok && tm.sm != nil {
		v, ok := tm.sm[s.S]
		if ok {
			return true, v
		}
	}
	h := key.Hash()
	for _, e := range tm.m[h] {
		if e.key.Equals(key) {
			return true, e.val
		}
	}
	return false, NIL
}

// ToPersistent freezes and returns a persistent ArrayMap or HashMap.
func (tm *TransientMap) ToPersistent() Object {
	tm.frozen = true
	if tm.count <= int(HASHMAP_THRESHOLD/2) {
		res := collectionConstruction.NewEmptyArrayMap()
		for k, v := range tm.sm {
			res.Add(coretypes.String{S: k}, v)
		}
		for _, bucket := range tm.m {
			for _, e := range bucket {
				res.Add(e.key, e.val)
			}
		}
		return res
	}
	res := EmptyHashMap
	for k, v := range tm.sm {
		res = res.Assoc(coretypes.String{S: k}, v).(*HashMap)
	}
	for _, bucket := range tm.m {
		for _, e := range bucket {
			res = res.Assoc(e.key, e.val).(*HashMap)
		}
	}
	return res
}

// MapToTransient creates a TransientMap from a Map.
func MapToTransient(m Map) *TransientMap {
	tm := &TransientMap{
		m: make(map[uint32][]mapEntry),
	}
	if m == nil {
		return tm
	}
	s := m.Seq()
	for !s.IsEmpty() {
		pair := s.First()
		// Map entries are seqable pairs (key val)
		if seq, ok := pair.(Seqable); ok {
			ps := seq.Seq()
			if !ps.IsEmpty() {
				key := ps.First()
				ps = ps.Rest()
				if !ps.IsEmpty() {
					val := ps.First()
					tm.AssocInPlace(key, val)
				}
			}
		}
		s = s.Rest()
	}
	return tm
}

// ---------- Joker procs ----------

var transientProcsOnce sync.Once

func init() {
	initTransientProcs()
}

// initTransientProcs registers transient, assoc!, conj!, persistent!, transient?, and pop! in the core namespace.
func initTransientProcs() {
	transientProcsOnce.Do(func() {
		ns := GLOBAL_ENV.CoreNamespace
		procs := []struct {
			name  string
			fn    func([]Object) Object
			pname string
		}{
			{"transient", procTransient, "procTransient"},
			{"assoc!", procAssocBang, "procAssocBang"},
			{"conj!", procConjBang, "procConjBang"},
			{"persistent!", procPersistentBang, "procPersistentBang"},
		}
		for _, p := range procs {
			sym := MakeSymbol(p.name)
			vr := ns.Intern(sym)
			vr.Value = Proc{Fn: p.fn, Name: p.pname}
			referToUser(sym, vr)
		}

		// transient?
		tqSym := MakeSymbol("transient?")
		tqVr := ns.Intern(tqSym)
		tqVr.Value = Proc{Name: "procTransientQ", Fn: procIsTransient}
		referToUser(tqSym, tqVr)

		// pop! — (pop! tv)
		popSym := MakeSymbol("pop!")
		popVr := ns.Intern(popSym)
		popVr.Value = Proc{Name: "procPopBang", Fn: procPopBang}
		referToUser(popSym, popVr)
	})
}

var procTransient = func(args []Object) Object {
	CheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *ArrayVector:
		return ToTransient(coll)
	case Map:
		return MapToTransient(coll)
	default:
		panic(RT.NewError("transient not supported on: " + coll.GetType().ToString(false)))
	}
}

var procAssocBang = func(args []Object) Object {
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

var procConjBang = func(args []Object) Object {
	CheckArity(args, 2, 3)
	switch coll := args[0].(type) {
	case *TransientVector:
		CheckArity(args, 2, 2)
		return coll.ConjInPlace(args[1])
	case *TransientMap:
		CheckArity(args, 3, 3)
		return coll.AssocInPlace(args[1], args[2])
	default:
		panic(RT.NewError("conj! requires a transient, got: " + coll.GetType().ToString(false)))
	}
}

var procPopBang = func(args []Object) Object {
	CheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *TransientVector:
		return coll.PopInPlace()
	default:
		panic(RT.NewError("pop! requires a transient vector, got: " + coll.GetType().ToString(false)))
	}
}

var procPersistentBang = func(args []Object) Object {
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

var procIsTransient = func(args []Object) Object {
	CheckArity(args, 1, 1)
	switch args[0].(type) {
	case *TransientVector, *TransientMap:
		return coretypes.Boolean{B: true}
	default:
		return coretypes.Boolean{B: false}
	}
}
