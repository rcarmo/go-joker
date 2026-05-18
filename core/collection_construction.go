package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	corecollections "github.com/rcarmo/go-joker/core/collections"
)

type CollectionConstructionAdapter = corecollections.ConstructionAdapter[coretypes.Object, coretypes.Seq, *List, *Vector, *ArrayVector, *ArrayMap, *HashMap, *MapSet]

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
	return corecollections.IndexedToString[coretypes.Object](v, escape)
}

func AreCountedIndexedEqual(v1, v2 coretypes.CountedIndexed) bool {
	return corecollections.IndexedEqual[coretypes.Object](v1, v2)
}

func CountedIndexedHash(v coretypes.CountedIndexed) uint32 {
	return corecollections.IndexedHash[coretypes.Object](v)
}

func CountedIndexedGet(v coretypes.CountedIndexed, key coretypes.Object) (bool, coretypes.Object) {
	switch key := key.(type) {
	case coretypes.Int:
		value, ok := corecollections.IndexedGet[coretypes.Object](v, key.I)
		return ok, value
	}
	return false, nil
}

func CountedIndexedCompare(v1, v2 coretypes.CountedIndexed) int {
	return corecollections.IndexedCompare[coretypes.Object](v1, v2, func(a, b coretypes.Object) int {
		return coretypes.EnsureObjectIsComparable(a, "").Compare(b)
	})
}

func CountedIndexedKvreduce(v coretypes.CountedIndexed, c coretypes.Callable, init coretypes.Object) coretypes.Object {
	return corecollections.IndexedKVReduce[coretypes.Object](v, init, func(res coretypes.Object, i int, value coretypes.Object) coretypes.Object {
		return call3(c, res, coretypes.Int{I: i}, value)
	})
}

func CountedIndexedPprint(v coretypes.CountedIndexed, w io.Writer, indent int) int {
	return corecollections.IndexedPprint[coretypes.Object](v, w, indent, pprintObject, writeIndent)
}

func CountedIndexedFormat(v coretypes.CountedIndexed, w io.Writer, indent int) int {
	return corecollections.IndexedFormat[coretypes.Object](v, w, indent, formatObject, maybeNewLine, isComment, writeIndent)
}

func CountedIndexedReduce(v coretypes.CountedIndexed, c coretypes.Callable) coretypes.Object {
	return corecollections.IndexedReduce[coretypes.Object](v, func() coretypes.Object { return call0(c) }, func(acc coretypes.Object, value coretypes.Object) coretypes.Object {
		return c.Call([]coretypes.Object{acc, value})
	})
}

func CountedIndexedReduceInit(v coretypes.CountedIndexed, c coretypes.Callable, init coretypes.Object) coretypes.Object {
	return corecollections.IndexedReduceInit[coretypes.Object](v, init, func(acc coretypes.Object, value coretypes.Object) coretypes.Object {
		return c.Call([]coretypes.Object{acc, value})
	})
}
