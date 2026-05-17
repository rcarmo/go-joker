package collections

// Pair exposes key/value entries to root map adapters without importing root
// Object or protocol types.
type Pair[K comparable, V any] struct {
	Key   K
	Value V
}

// EqualMaps compares map-like values using caller-provided count, iteration,
// lookup, and equality semantics. Root core supplies Object equality and concrete
// map adapters; this package owns only the root-independent comparison loop.
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
