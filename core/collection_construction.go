package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	corecollections "github.com/rcarmo/go-joker/core/collections"
)

type CollectionConstructionAdapter = corecollections.ConstructionAdapter[Object, Seq, *List, *Vector, *ArrayVector, *ArrayMap, *HashMap, *MapSet]

var collectionConstruction = CollectionConstructionAdapter{
	EmptyList:        func() *List { return EmptyList },
	ListFrom:         NewListFrom,
	EmptyVector:      EmptyVector,
	VectorFrom:       NewVectorFrom,
	VectorFromSeq:    NewVectorFromSeq,
	EmptyArrayVector: EmptyArrayVector,
	ArrayVectorFrom:  NewArrayVectorFrom,
	EmptyArrayMap:    EmptyArrayMap,
	HashMapFrom:      NewHashMap,
	EmptySet:         EmptySet,
	SetFromSeq:       NewSetFromSeq,
}

func CountedIndexedToString(v coretypes.CountedIndexed, escape bool) string {
	return corecollections.IndexedToString[Object](v, escape)
}

func AreCountedIndexedEqual(v1, v2 coretypes.CountedIndexed) bool {
	return corecollections.IndexedEqual[Object](v1, v2)
}

func CountedIndexedHash(v coretypes.CountedIndexed) uint32 {
	return corecollections.IndexedHash[Object](v)
}

func CountedIndexedGet(v coretypes.CountedIndexed, key Object) (bool, Object) {
	switch key := key.(type) {
	case coretypes.Int:
		value, ok := corecollections.IndexedGet[Object](v, key.I)
		return ok, value
	}
	return false, nil
}

func CountedIndexedCompare(v1, v2 coretypes.CountedIndexed) int {
	return corecollections.IndexedCompare[Object](v1, v2, func(a, b Object) int {
		return EnsureObjectIsComparable(a, "").Compare(b)
	})
}

func CountedIndexedKvreduce(v coretypes.CountedIndexed, c Callable, init Object) Object {
	return corecollections.IndexedKVReduce[Object](v, init, func(res Object, i int, value Object) Object {
		return call3(c, res, coretypes.Int{I: i}, value)
	})
}

func CountedIndexedPprint(v coretypes.CountedIndexed, w io.Writer, indent int) int {
	return corecollections.IndexedPprint[Object](v, w, indent, pprintObject, writeIndent)
}

func CountedIndexedFormat(v coretypes.CountedIndexed, w io.Writer, indent int) int {
	return corecollections.IndexedFormat[Object](v, w, indent, formatObject, maybeNewLine, isComment, writeIndent)
}

func CountedIndexedReduce(v coretypes.CountedIndexed, c Callable) Object {
	return corecollections.IndexedReduce[Object](v, func() Object { return call0(c) }, func(acc Object, value Object) Object {
		return c.Call([]Object{acc, value})
	})
}

func CountedIndexedReduceInit(v coretypes.CountedIndexed, c Callable, init Object) Object {
	return corecollections.IndexedReduceInit[Object](v, init, func(acc Object, value Object) Object {
		return c.Call([]Object{acc, value})
	})
}
