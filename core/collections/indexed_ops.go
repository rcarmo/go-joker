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

func IndexedHash[T coretypes.Object](v IndexedView[T]) uint32 {
	h := hashutil.New32()
	for i := 0; i < v.Count(); i++ {
		h.Write(hashutil.Uint32Bytes(v.At(i).Hash()))
	}
	return h.Sum32()
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
