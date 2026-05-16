package reader

import "fmt"

// DecodeStringEscapeCode parses a string unicode/octal escape after the caller
// has collected its digit token. It keeps token-length/base mechanics in the
// reader package while callers continue to own positioned error construction.
func DecodeStringEscapeCode(str string, length int, base int, exactLength bool) (rune, error) {
	if exactLength && !HasExactLength(str, length) {
		return 0, fmt.Errorf("invalid character length: %d, should be: %d", len(str), length)
	}
	r, err := ParseUnicodeCode(str, base)
	if err != nil {
		return 0, fmt.Errorf("invalid unicode code: %s", str)
	}
	return r, nil
}
