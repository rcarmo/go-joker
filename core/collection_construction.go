package core

import (
	"io"

	corecollections "github.com/rcarmo/go-joker/core/collections"
)

// CollectionConstructionAdapter is the narrow root-owned construction surface
// for collection values. Future extraction of vectors/maps/sets should route
// through this surface instead of scattering concrete constructor calls across
// evaluator code.
type CollectionConstructionAdapter struct{}

var collectionConstruction CollectionConstructionAdapter

func (CollectionConstructionAdapter) EmptyVector() *Vector {
	return EmptyVector()
}

func (CollectionConstructionAdapter) VectorFrom(objs ...Object) *Vector {
	return NewVectorFrom(objs...)
}

func (CollectionConstructionAdapter) VectorFromSeq(seq Seq) *Vector {
	return NewVectorFromSeq(seq)
}

func (CollectionConstructionAdapter) EmptyArrayVector() *ArrayVector {
	return EmptyArrayVector()
}

func (CollectionConstructionAdapter) ArrayVectorFrom(objs ...Object) *ArrayVector {
	return NewArrayVectorFrom(objs...)
}

func (CollectionConstructionAdapter) EmptyArrayMap() *ArrayMap {
	return EmptyArrayMap()
}

func (CollectionConstructionAdapter) HashMapFrom(keyvals ...Object) *HashMap {
	return NewHashMap(keyvals...)
}

func (CollectionConstructionAdapter) EmptySet() *MapSet {
	return EmptySet()
}

func (CollectionConstructionAdapter) SetFromSeq(seq Seq) *MapSet {
	return NewSetFromSeq(seq)
}

func CountedIndexedToString(v CountedIndexed, escape bool) string {
	return corecollections.IndexedToString[Object](v, escape)
}

func AreCountedIndexedEqual(v1, v2 CountedIndexed) bool {
	return corecollections.IndexedEqual[Object](v1, v2)
}

func CountedIndexedHash(v CountedIndexed) uint32 {
	return corecollections.IndexedHash[Object](v)
}

func CountedIndexedGet(v CountedIndexed, key Object) (bool, Object) {
	switch key := key.(type) {
	case Int:
		value, ok := corecollections.IndexedGet[Object](v, key.I)
		return ok, value
	}
	return false, nil
}

func CountedIndexedCompare(v1, v2 CountedIndexed) int {
	return corecollections.IndexedCompare[Object](v1, v2, func(a, b Object) int {
		return EnsureObjectIsComparable(a, "").Compare(b)
	})
}

func CountedIndexedKvreduce(v CountedIndexed, c Callable, init Object) Object {
	return corecollections.IndexedKVReduce[Object](v, init, func(res Object, i int, value Object) Object {
		return call3(c, res, Int{I: i}, value)
	})
}

func CountedIndexedPprint(v CountedIndexed, w io.Writer, indent int) int {
	return corecollections.IndexedPprint[Object](v, w, indent, pprintObject, writeIndent)
}

func CountedIndexedFormat(v CountedIndexed, w io.Writer, indent int) int {
	return corecollections.IndexedFormat[Object](v, w, indent, formatObject, maybeNewLine, isComment, writeIndent)
}

func CountedIndexedReduce(v CountedIndexed, c Callable) Object {
	return corecollections.IndexedReduce[Object](v, func() Object { return call0(c) }, func(acc Object, value Object) Object {
		return c.Call([]Object{acc, value})
	})
}

func CountedIndexedReduceInit(v CountedIndexed, c Callable, init Object) Object {
	return corecollections.IndexedReduceInit[Object](v, init, func(acc Object, value Object) Object {
		return c.Call([]Object{acc, value})
	})
}
