package core

import "strconv"

// int_cache.go — cached string representations for small integers.
//
// Avoids repeated strconv.Itoa allocations for common integer values
// (0-255 are extremely frequent in Clojure: loop counters, indices,
// collection sizes, ASCII chars).

const intCacheMin = -128
const intCacheMax = 1024
const intCacheSize = intCacheMax - intCacheMin

var intStringCache [intCacheSize]string

func init() {
	for i := range intStringCache {
		intStringCache[i] = strconv.Itoa(i + intCacheMin)
	}
}

// intToString returns the string representation of an integer,
// using the cache for small values.
func intToString(i int) string {
	idx := i - intCacheMin
	if idx >= 0 && idx < intCacheSize {
		return intStringCache[idx]
	}
	return strconv.Itoa(i)
}
