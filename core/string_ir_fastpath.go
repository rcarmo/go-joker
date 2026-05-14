package core

import (
	"fmt"

	corestr "github.com/rcarmo/go-joker/core/string"
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
	if r, length, ok := corestr.NthRune(s, i); ok {
		return Char{Ch: r}
	} else {
		panic(RT.NewError(fmt.Sprintf("Index %d exceeds string's length %d", i, length)))
	}
}
