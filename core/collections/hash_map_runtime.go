package collections

type Iterator[T any] interface {
	HasNext() bool
	Next() T
}

func NodeArrayAdvance[T any](array []interface{}, i int, iterFor func(any) Iterator[T], makeEntry func(key any, value any) T) (nextI int, entry T, hasEntry bool, nested Iterator[T], hasNested bool) {
	for i < len(array) {
		key := array[i]
		nodeOrVal := array[i+1]
		i += 2
		if key != nil {
			return i, makeEntry(key, nodeOrVal), true, nil, false
		}
		if nodeOrVal != nil {
			iter := iterFor(nodeOrVal)
			if iter != nil && iter.HasNext() {
				return i, entry, false, iter, true
			}
		}
	}
	return i, entry, false, nil, false
}

func ArrayNodeIterHasNext[N any, T any](nodes []N, i int, nested Iterator[T], isNil func(N) bool, iterFor func(N) Iterator[T]) (nextI int, nextNested Iterator[T], has bool) {
	for {
		if nested != nil {
			if nested.HasNext() {
				return i, nested, true
			}
			nested = nil
		}
		if i >= len(nodes) {
			return i, nil, false
		}
		node := nodes[i]
		i++
		if !isNil(node) {
			nested = iterFor(node)
		}
	}
}
