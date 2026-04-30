package core

import (
	"fmt"
	"unicode/utf8"
)

// stringNthFast returns the i-th rune of s with an ASCII-prefix fast path.
//
// Joker's string indexing is by rune index. For ASCII prefixes, byte and rune
// offsets are identical, which covers the common CLBG/gi text-processing hot
// path without changing Unicode semantics. If a non-ASCII byte appears before
// the requested index, this falls back to the Unicode-correct range walk.
func stringNthFast(s string, i int) Object {
	if i < 0 {
		panic(RT.NewError(fmt.Sprintf("Negative index: %d", i)))
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
			return Char{Ch: rune(s[i])}
		}
	}
	idx := 0
	for _, r := range s {
		if idx == i {
			return Char{Ch: r}
		}
		idx++
	}
	panic(RT.NewError(fmt.Sprintf("Index %d exceeds string's length %d", i, idx)))
}
