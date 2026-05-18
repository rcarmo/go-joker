package string

// ObjectCache maps ASCII runes to caller-owned values. It lets root runtime code
// keep object-specific caching out of the string helper package mechanics.
type ObjectCache[T any] struct {
	ascii [128]T
}

func NewObjectCache[T any](makeValue func(rune) T) ObjectCache[T] {
	var cache ObjectCache[T]
	for i := 0; i < len(cache.ascii); i++ {
		cache.ascii[i] = makeValue(rune(i))
	}
	return cache
}

func (c *ObjectCache[T]) Lookup(ch rune) (T, bool) {
	if ch >= 0 && ch < rune(len(c.ascii)) {
		return c.ascii[ch], true
	}
	var zero T
	return zero, false
}
