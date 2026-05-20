package collections

import (
	"fmt"
	"io"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

var (
	VECTOR_THRESHOLD int = 16
)

type (
	ArrayVector struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		Arr []coretypes.Object
	}
)

func (v *ArrayVector) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	v.Info = info
	return v
}

func (v *ArrayVector) WithMeta(meta coretypes.Map) coretypes.Object {
	res := WithMergedMeta(*v, v.Meta, meta, func(av *ArrayVector, m coretypes.Map) { av.Meta = m })
	return &res
}

func (v *ArrayVector) Clone() *ArrayVector {
	res := ArrayVector{Arr: CloneSlice(v.Arr)}
	res.Meta = v.Meta
	return &res
}

func (v *ArrayVector) Conjoin(obj coretypes.Object) coretypes.Vec {
	if arr, overflow := VectorConjoinCopy(v.Arr, obj, VECTOR_THRESHOLD); !overflow {
		res := *v
		res.Arr = arr
		return &res
	}
	res := NewVectorFrom(v.Arr...)
	res = res.Conjoin(obj)
	res.Meta = v.Meta
	return res
}

func (v *ArrayVector) Append(obj coretypes.Object) {
	v.Arr = VectorAppendInPlace(v.Arr, obj)
}

func (v *ArrayVector) At(i int) coretypes.Object {
	return v.Arr[i]
}

func (v *ArrayVector) ToString(escape bool) string {
	return IndexedToString[coretypes.Object](v, escape)
}

func (v *ArrayVector) Equals(other interface{}) bool {
	if v == other {
		return true
	}
	switch other := other.(type) {
	case coretypes.CountedIndexed:
		return IndexedEqual[coretypes.Object](v, other)
	default:
		return coretypes.IsSeqEqual(v.Seq(), other)
	}
}

func (v *ArrayVector) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.ArrayVector
}

func (v *ArrayVector) Hash() uint32 {
	return IndexedHash[coretypes.Object](v)
}

func (v *ArrayVector) Seq() coretypes.Seq {
	return &VectorSeq{Vector: v, Index: 0}
}

func (v *ArrayVector) Conj(obj coretypes.Object) coretypes.Conjable {
	return v.Conjoin(obj)
}

func (v *ArrayVector) Count() int {
	return VectorCount(v.Arr)
}

func (v *ArrayVector) Nth(i int) coretypes.Object {
	if value, ok := VectorNth(v.Arr, i); ok {
		return value
	}
	panic(coretypes.RuntimeError(fmt.Sprintf("Index %d is out of bounds [0..%d]", i, v.Count()-1)))
}

func (v *ArrayVector) TryNth(i int, d coretypes.Object) coretypes.Object {
	return VectorTryNth(v.Arr, i, d)
}

func (v *ArrayVector) SequentialMarker() {}

func (v *ArrayVector) Compare(other coretypes.Object) int {
	v2 := coretypes.EnsureObjectIsCountedIndexed(coretypes.RootObject(other), "Cannot compare coretypes.Vector: %s")
	return IndexedCompare[coretypes.Object](v, v2, func(a, b coretypes.Object) int { return coretypes.EnsureObjectIsComparable(a, "").Compare(b) })
}

func (v *ArrayVector) Peek() coretypes.Object {
	if value, ok := VectorPeek(v.Arr); ok {
		return value
	}
	return coretypes.RuntimeNil
}

func (v *ArrayVector) Pop() coretypes.Stack {
	next, ok := VectorPop(v.Arr)
	if !ok {
		panic(coretypes.RuntimeError("Can't pop empty vector"))
	}
	res := *v
	res.Arr = next
	return &res
}

func (v *ArrayVector) Get(key coretypes.Object) (bool, coretypes.Object) {
	return IndexedGetByObject[coretypes.Object](v, key)
}

func (v *ArrayVector) EntryAt(key coretypes.Object) coretypes.Object {
	if ok, val := v.Get(key); ok {
		return NewArrayVectorFrom(key, val)
	}
	return nil
}

func (v *ArrayVector) Assoc(key, val coretypes.Object) coretypes.Associative {
	i, ok := IndexFromObject(key)
	if !ok {
		panic(coretypes.RuntimeError("Key must be integer"))
	}
	next, appendMode, valid := VectorAssoc(v.Arr, i, val)
	if !valid {
		panic(coretypes.RuntimeError((fmt.Sprintf("Index %d is out of bounds [0..%d]", i, v.Count()))))
	}
	if appendMode {
		return v.Conjoin(val)
	}
	res := *v
	res.Arr = next
	return &res
}

func (v *ArrayVector) Rseq() coretypes.Seq {
	return &VectorRSeq{Vector: v, Index: v.Count() - 1}
}

func (v *ArrayVector) Call(args []coretypes.Object) coretypes.Object {
	if len(args) != 1 {
		coretypes.RuntimePanicArityMinMax(len(args), 1, 1)
	}
	i, ok := IndexFromObject(args[0])
	if !ok {
		panic(coretypes.RuntimeError("Key must be integer"))
	}
	return v.Nth(i)
}

func EmptyArrayVector() *ArrayVector {
	return &ArrayVector{}
}

func (v *ArrayVector) Empty() coretypes.Collection {
	return EmptyArrayVector()
}

func (v *ArrayVector) KVReduce(c coretypes.Callable, init coretypes.Object) coretypes.Object {
	return IndexedKVReduce[coretypes.Object](v, init, func(res coretypes.Object, i int, value coretypes.Object) coretypes.Object {
		return c.Call([]coretypes.Object{res, coretypes.Int{I: i}, value})
	})
}

func (v *ArrayVector) Pprint(w io.Writer, indent int) int {
	return IndexedPprint[coretypes.Object](v, w, indent, coretypes.RuntimePprintObject, coretypes.RuntimeWriteIndent)
}

func (v *ArrayVector) Format(w io.Writer, indent int) int {
	return IndexedFormat[coretypes.Object](v, w, indent, coretypes.RuntimeFormatObject, coretypes.RuntimeMaybeNewLine, coretypes.RuntimeIsComment, coretypes.RuntimeWriteIndent)
}

func NewArrayVectorFrom(objs ...coretypes.Object) *ArrayVector {
	n := len(objs)
	if n == 0 {
		return EmptyArrayVector()
	}
	return &ArrayVector{Arr: FromValues(objs...)}
}

func (v *ArrayVector) Reduce(c coretypes.Callable) coretypes.Object {
	return IndexedReduce[coretypes.Object](v, func() coretypes.Object { return c.Call(nil) }, func(acc coretypes.Object, value coretypes.Object) coretypes.Object {
		return c.Call([]coretypes.Object{acc, value})
	})
}

func (v *ArrayVector) ReduceInit(c coretypes.Callable, init coretypes.Object) coretypes.Object {
	return IndexedReduceInit[coretypes.Object](v, init, func(acc coretypes.Object, value coretypes.Object) coretypes.Object {
		return c.Call([]coretypes.Object{acc, value})
	})
}
