package types

import "fmt"

type TransientMapEntry struct {
	Key Object
	Val Object
}

type TransientVector struct {
	Arr    []Object
	Frozen bool
}

type TransientMap struct {
	M      map[uint32][]TransientMapEntry
	SM     map[string]Object
	CountN int
	Frozen bool
}

var TransientMutationError func() any
var TransientVectorIndexTypeError func(Object) any
var TransientVectorToPersistent func([]Object) Object
var TransientMapToPersistent func(*TransientMap) Object

func (tv *TransientVector) ToString(escape bool) string   { return "#<transient-vector>" }
func (tv *TransientVector) Equals(other interface{}) bool { return tv == other }
func (tv *TransientVector) GetInfo() *ObjectInfo          { return nil }
func (tv *TransientVector) WithInfo(*ObjectInfo) Object   { return tv }
func (tv *TransientVector) GetType() *Type                { return RuntimeTypes.ArrayVector }
func (tv *TransientVector) Hash() uint32                  { return 0 }
func (tv *TransientVector) Count() int                    { return len(tv.Arr) }

func panicTransientMutation() {
	if TransientMutationError != nil {
		panic(TransientMutationError())
	}
	panic("Cannot mutate a frozen transient")
}

func (tv *TransientVector) checkFrozen() {
	if tv.Frozen {
		panicTransientMutation()
	}
}

func (tv *TransientVector) AssocInPlace(key, val Object) *TransientVector {
	tv.checkFrozen()
	idxObj, ok := key.(Int)
	if !ok {
		if TransientVectorIndexTypeError != nil {
			panic(TransientVectorIndexTypeError(key))
		}
		panic(fmt.Sprintf("Key must be Int, got %T", key))
	}
	idx := idxObj.I
	if idx >= 0 && idx < len(tv.Arr) {
		tv.Arr[idx] = val
	} else if idx == len(tv.Arr) {
		tv.Arr = append(tv.Arr, val)
	}
	return tv
}

func (tv *TransientVector) ConjInPlace(val Object) *TransientVector {
	tv.checkFrozen()
	tv.Arr = append(tv.Arr, val)
	return tv
}
func (tv *TransientVector) PopInPlace() *TransientVector {
	tv.checkFrozen()
	if len(tv.Arr) > 0 {
		tv.Arr = tv.Arr[:len(tv.Arr)-1]
	}
	return tv
}
func (tv *TransientVector) At(i int) Object { return tv.Nth(i) }
func (tv *TransientVector) Nth(i int) Object {
	if i >= 0 && i < len(tv.Arr) {
		return tv.Arr[i]
	}
	panic(RuntimeError(fmt.Sprintf("Index %d is out of bounds [0..%d]", i, len(tv.Arr)-1)))
}
func (tv *TransientVector) TryNth(i int, d Object) Object {
	if i >= 0 && i < len(tv.Arr) {
		return tv.Arr[i]
	}
	return d
}
func (tv *TransientVector) Get(key Object) (bool, Object) {
	if idx, ok := key.(Int); ok {
		if idx.I >= 0 && idx.I < len(tv.Arr) {
			return true, tv.Arr[idx.I]
		}
	}
	return false, RuntimeNil
}
func (tv *TransientVector) ToPersistent() Object {
	tv.Frozen = true
	if TransientVectorToPersistent == nil {
		return RuntimeNil
	}
	arr := make([]Object, len(tv.Arr))
	copy(arr, tv.Arr)
	return TransientVectorToPersistent(arr)
}

func ToTransient(v []Object) *TransientVector {
	arr := make([]Object, len(v))
	copy(arr, v)
	return &TransientVector{Arr: arr}
}

func (tm *TransientMap) ToString(escape bool) string   { return "#<transient-map>" }
func (tm *TransientMap) Equals(other interface{}) bool { return tm == other }
func (tm *TransientMap) GetInfo() *ObjectInfo          { return nil }
func (tm *TransientMap) WithInfo(*ObjectInfo) Object   { return tm }
func (tm *TransientMap) GetType() *Type                { return RuntimeTypes.ArrayMap }
func (tm *TransientMap) Hash() uint32                  { return 0 }
func (tm *TransientMap) Count() int                    { return tm.CountN }
func (tm *TransientMap) checkFrozen() {
	if tm.Frozen {
		panicTransientMutation()
	}
}

func (tm *TransientMap) AssocInPlace(key, val Object) *TransientMap {
	tm.checkFrozen()
	if s, ok := key.(String); ok {
		if tm.SM == nil {
			tm.SM = make(map[string]Object)
		}
		if _, exists := tm.SM[s.S]; !exists {
			tm.CountN++
		}
		tm.SM[s.S] = val
		return tm
	}
	if tm.M == nil {
		tm.M = make(map[uint32][]TransientMapEntry)
	}
	h := key.Hash()
	bucket := tm.M[h]
	for i, e := range bucket {
		if e.Key.Equals(key) {
			tm.M[h][i].Val = val
			return tm
		}
	}
	tm.M[h] = append(bucket, TransientMapEntry{Key: key, Val: val})
	tm.CountN++
	return tm
}
func (tm *TransientMap) Get(key Object) (bool, Object) {
	if s, ok := key.(String); ok && tm.SM != nil {
		v, ok := tm.SM[s.S]
		if ok {
			return true, v
		}
	}
	h := key.Hash()
	for _, e := range tm.M[h] {
		if e.Key.Equals(key) {
			return true, e.Val
		}
	}
	return false, RuntimeNil
}
func (tm *TransientMap) ToPersistent() Object {
	tm.Frozen = true
	if TransientMapToPersistent == nil {
		return RuntimeNil
	}
	return TransientMapToPersistent(tm)
}

func MapToTransient(m Map) *TransientMap {
	tm := &TransientMap{M: make(map[uint32][]TransientMapEntry)}
	if m == nil {
		return tm
	}
	s := m.Seq()
	for !s.IsEmpty() {
		pair := s.First()
		if seq, ok := pair.(Seqable); ok {
			ps := seq.Seq()
			if !ps.IsEmpty() {
				key := ps.First()
				ps = ps.Rest()
				if !ps.IsEmpty() {
					tm.AssocInPlace(key, ps.First())
				}
			}
		}
		s = s.Rest()
	}
	return tm
}
