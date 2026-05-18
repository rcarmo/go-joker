package collections

import "sort"

type KV[K any, V any] struct {
	Key K
	Val V
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
