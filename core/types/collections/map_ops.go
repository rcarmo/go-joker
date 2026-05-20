package collections

import (
	"fmt"
	"io"
	"sort"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type Pair[K comparable, V any] struct {
	Key   K
	Value V
}

type KV[K any, V any] struct {
	Key K
	Val V
}

type ArrayBackedMap[T coretypes.Object] interface {
	ArrayItems() []T
}

func EqualMaps[K comparable, V any](countA int, countB int, iterA func(func(Pair[K, V]) bool), getB func(K) (V, bool), equal func(V, V) bool) bool {
	if countA != countB {
		return false
	}
	ok := true
	iterA(func(p Pair[K, V]) bool {
		value, found := getB(p.Key)
		if !found || !equal(p.Value, value) {
			ok = false
			return false
		}
		return true
	})
	return ok
}

func MapConj(m coretypes.Map, obj coretypes.Object, errf func(string) any) coretypes.Conjable {
	switch o := obj.(type) {
	case coretypes.Vec:
		if o.Count() != 2 {
			panic(errf("Vector argument to map's conj must be a vector with two elements"))
		}
		return m.Assoc(o.At(0), o.At(1))
	case coretypes.Map:
		return m.Merge(o)
	default:
		panic(errf("Argument to map's conj must be a vector with two elements or a map"))
	}
}

func CallMap(m coretypes.Map, args []coretypes.Object, checkArity func([]coretypes.Object, int, int), nilObj coretypes.Object) coretypes.Object {
	checkArity(args, 1, 2)
	if ok, v := m.Get(args[0]); ok {
		return v
	}
	if len(args) == 2 {
		return args[1]
	}
	return nilObj
}

func MapEquals(m coretypes.Map, other interface{}) bool {
	if m == other {
		return true
	}
	otherMap, ok := other.(coretypes.Map)
	if !ok || m.Count() != otherMap.Count() {
		return false
	}
	if am, ok := m.(ArrayBackedMap[coretypes.Object]); ok {
		if otherAM, ok := otherMap.(ArrayBackedMap[coretypes.Object]); ok {
			return ArrayMapEquals(am.ArrayItems(), otherAM.ArrayItems())
		}
	}
	if otherAM, ok := otherMap.(ArrayBackedMap[coretypes.Object]); ok {
		for items, i := otherAM.ArrayItems(), 0; i < len(items); i += 2 {
			key, value := items[i], items[i+1]
			found, current := m.Get(key)
			if !found || !current.Equals(value) {
				return false
			}
		}
		return true
	}
	return EqualMaps(
		m.Count(),
		otherMap.Count(),
		func(yield func(Pair[coretypes.Object, coretypes.Object]) bool) {
			for iter := m.Iter(); iter.HasNext(); {
				p := iter.Next()
				if !yield(Pair[coretypes.Object, coretypes.Object]{Key: p.Key, Value: p.Value}) {
					return
				}
			}
		},
		func(key coretypes.Object) (coretypes.Object, bool) {
			found, value := otherMap.Get(key)
			return value, found
		},
		func(a coretypes.Object, b coretypes.Object) bool {
			return b.Equals(a)
		},
	)
}

func MapToString(m coretypes.Map, escape bool) string {
	return FormatPairDelimited(
		"{",
		"}",
		" ",
		", ",
		func(yield func(Pair[coretypes.Object, coretypes.Object]) bool) {
			for iter := m.Iter(); iter.HasNext(); {
				p := iter.Next()
				if !yield(Pair[coretypes.Object, coretypes.Object]{Key: p.Key, Value: p.Value}) {
					return
				}
			}
		},
		func(key coretypes.Object) string { return key.ToString(escape) },
		func(value coretypes.Object) string { return value.ToString(escape) },
	)
}

func PprintMap(m coretypes.Map, w io.Writer, indent int, pprint func(coretypes.Object, int, io.Writer) int, writeIndent func(io.Writer, int)) int {
	i := indent + 1
	fmt.Fprint(w, "{")
	if m.Count() > 0 {
		for iter := m.Iter(); ; {
			p := iter.Next()
			i = pprint(p.Key, indent+1, w)
			fmt.Fprint(w, " ")
			i = pprint(p.Value, i+1, w)
			if iter.HasNext() {
				fmt.Fprint(w, "\n")
				writeIndent(w, indent+1)
			} else {
				break
			}
		}
	}
	fmt.Fprint(w, "}")
	return i + 1
}

func CompareObjectsDefault(a, b coretypes.Object) int {
	switch av := a.(type) {
	case coretypes.Int:
		if bv, ok := b.(coretypes.Int); ok {
			if av.I < bv.I {
				return -1
			}
			if av.I > bv.I {
				return 1
			}
			return 0
		}
	case coretypes.Double:
		if bv, ok := b.(coretypes.Double); ok {
			if av.D < bv.D {
				return -1
			}
			if av.D > bv.D {
				return 1
			}
			return 0
		}
	case coretypes.String:
		if bv, ok := b.(coretypes.String); ok {
			if av.S < bv.S {
				return -1
			}
			if av.S > bv.S {
				return 1
			}
			return 0
		}
	case coretypes.Keyword:
		if bv, ok := b.(coretypes.Keyword); ok {
			return compareStrings(av.ToString(false), bv.ToString(false))
		}
	case coretypes.Symbol:
		if bv, ok := b.(coretypes.Symbol); ok {
			return compareStrings(av.ToString(false), bv.ToString(false))
		}
	}
	return compareStrings(a.ToString(false), b.ToString(false))
}

func compareStrings(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func FlatToKVs[T any](flat []T) []KV[T, T] {
	kvs := make([]KV[T, T], len(flat)/2)
	for i := 0; i < len(flat); i += 2 {
		kvs[i/2] = KV[T, T]{Key: flat[i], Val: flat[i+1]}
	}
	return kvs
}
func SortKVsBy[K any, V any](kvs []KV[K, V], less func(a, b K) bool) {
	sort.SliceStable(kvs, func(i, j int) bool { return less(kvs[i].Key, kvs[j].Key) })
}
func SortBy[T any](values []T, less func(a, b T) bool) {
	sort.Slice(values, func(i, j int) bool { return less(values[i], values[j]) })
}
func Reverse[T any](values []T) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}
