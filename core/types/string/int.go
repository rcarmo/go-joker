package string

import "strconv"

const intCacheMin = -128
const intCacheMax = 1024
const intCacheSize = intCacheMax - intCacheMin

var intStringCache [intCacheSize]string

func init() {
	for i := range intStringCache {
		intStringCache[i] = strconv.Itoa(i + intCacheMin)
	}
}

// Int returns the string representation of an integer,
// using the cache for small values.
func Int(i int) string {
	idx := i - intCacheMin
	if idx >= 0 && idx < intCacheSize {
		return intStringCache[idx]
	}
	return strconv.Itoa(i)
}
