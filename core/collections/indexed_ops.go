package collections

import (
	"strings"

	"github.com/rcarmo/go-joker/core/hashutil"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

type IndexedView[T coretypes.Object] interface {
	Count() int
	At(int) T
}

func IndexedToString[T coretypes.Object](v IndexedView[T], escape bool) string {
	var b strings.Builder
	b.WriteByte('[')
	cnt := v.Count()
	for i := 0; i < cnt; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(v.At(i).ToString(escape))
	}
	b.WriteByte(']')
	return b.String()
}

func IndexedEqual[T coretypes.Object](v1, v2 IndexedView[T]) bool {
	if v1.Count() != v2.Count() {
		return false
	}
	for i := 0; i < v1.Count(); i++ {
		if !v1.At(i).Equals(v2.At(i)) {
			return false
		}
	}
	return true
}

func IndexedGet[T coretypes.Object](v IndexedView[T], index int) (T, bool) {
	var zero T
	if index < 0 || index >= v.Count() {
		return zero, false
	}
	return v.At(index), true
}

func IndexedHash[T coretypes.Object](v IndexedView[T]) uint32 {
	h := hashutil.New32()
	for i := 0; i < v.Count(); i++ {
		h.Write(hashutil.Uint32Bytes(v.At(i).Hash()))
	}
	return h.Sum32()
}

func IndexedKVReduce[T coretypes.Object, R any](v IndexedView[T], init R, reduce func(R, int, T) R) R {
	res := init
	for i := 0; i < v.Count(); i++ {
		res = reduce(res, i, v.At(i))
	}
	return res
}

func IndexedReduce[T coretypes.Object, R any](v IndexedView[T], zero func() R, reduce func(R, T) R) R {
	switch v.Count() {
	case 0:
		return zero()
	case 1:
		return any(v.At(0)).(R)
	default:
		acc := reduce(any(v.At(0)).(R), v.At(1))
		for i := 2; i < v.Count(); i++ {
			acc = reduce(acc, v.At(i))
		}
		return acc
	}
}

func IndexedReduceInit[T coretypes.Object, R any](v IndexedView[T], init R, reduce func(R, T) R) R {
	acc := init
	for i := 0; i < v.Count(); i++ {
		acc = reduce(acc, v.At(i))
	}
	return acc
}

func IndexedCompare[T coretypes.Object](v1, v2 IndexedView[T], compare func(T, T) int) int {
	if v1.Count() > v2.Count() {
		return 1
	}
	if v1.Count() < v2.Count() {
		return -1
	}
	for i := 0; i < v1.Count(); i++ {
		if c := compare(v1.At(i), v2.At(i)); c != 0 {
			return c
		}
	}
	return 0
}
