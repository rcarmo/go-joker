package reader

import "github.com/rcarmo/go-joker/core/numutil"

// ParseUnicodeCode parses a reader unicode escape/code point using base and
// returns the decoded rune. Callers keep reader-position/error construction in
// their own package.
func ParseUnicodeCode(str string, base int) (rune, error) {
	i, err := numutil.ParseInt(str, base, 32)
	if err != nil {
		return 0, err
	}
	return rune(i), nil
}

// HasExactLength reports whether str has exactly length bytes. Reader escape
// validation historically uses byte length because escape digits are ASCII.
func HasExactLength(str string, length int) bool {
	return len(str) == length
}
