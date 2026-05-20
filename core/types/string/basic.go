package string

import (
	"strconv"
	stringsdk "strings"
	"sync"
)

var ascii [128]string
var asciiCache sync.Map // string -> bool

const intCacheMin = -128
const intCacheMax = 1024
const intCacheSize = intCacheMax - intCacheMin

var intStringCache [intCacheSize]string

func init() {
	for i := 0; i < len(ascii); i++ {
		ascii[i] = string(rune(i))
	}
	for i := range intStringCache {
		intStringCache[i] = strconv.Itoa(i + intCacheMin)
	}
}

func IsASCII(s string) bool {
	if len(s) <= 8 {
		for i := 0; i < len(s); i++ {
			if s[i] >= 0x80 {
				return false
			}
		}
		return true
	}
	if v, ok := asciiCache.Load(s); ok {
		return v.(bool)
	}
	result := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			result = false
			break
		}
	}
	asciiCache.Store(s, result)
	return result
}

func String(ch rune) string {
	if ch >= 0 && ch < rune(len(ascii)) {
		return ascii[ch]
	}
	return string(ch)
}

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

func Hash(s string) uint32 {
	h := uint32(0)
	for i := 0; i < len(s) && i < 32; i++ {
		h = h*31 + uint32(s[i])
	}
	return h
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

// JoinDotted joins path/name parts using a dot separator.
func JoinDotted(parts []string) string {
	return stringsdk.Join(parts, ".")
}
