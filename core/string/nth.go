package string

import "unicode/utf8"

// NthRune returns the i-th rune of s with an ASCII-prefix fast path.
// The returned length is the total rune length observed when i is out of range.
func NthRune(s string, i int) (rune, int, bool) {
	if i < 0 {
		return 0, 0, false
	}
	if i < len(s) {
		asciiPrefix := true
		for j := 0; j <= i; j++ {
			if s[j] >= utf8.RuneSelf {
				asciiPrefix = false
				break
			}
		}
		if asciiPrefix {
			return rune(s[i]), 0, true
		}
	}
	idx := 0
	for _, r := range s {
		if idx == i {
			return r, 0, true
		}
		idx++
	}
	return 0, idx, false
}
