package reader

import (
	"strings"

	"github.com/rcarmo/go-joker/core/numutil"
)

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

// ParseExactUnicodeCode validates a fixed-width ASCII unicode/octal token and
// parses it using base. Callers own reader-position/error construction.
func ParseExactUnicodeCode(str string, length int, base int) (rune, bool) {
	if !HasExactLength(str, length) {
		return 0, false
	}
	r, err := ParseUnicodeCode(str, base)
	if err != nil {
		return 0, false
	}
	return r, true
}

// ScanStringEscapeCode consumes up to length runes for a string escape code,
// stopping early at an unconsumed double quote. The terminating/lookahead rune
// is pushed back through Unget, matching root reader behavior.
func ScanStringEscapeCode(r interface {
	Get() rune
	Unget()
}, initial rune, length int) string {
	ch := initial
	var b strings.Builder
	for i := 0; i < length && ch != '"'; i++ {
		b.WriteRune(ch)
		ch = r.Get()
	}
	r.Unget()
	return b.String()
}
