package reader

import "strings"

// ScanRegexLiteral consumes a regex literal body through the closing quote,
// preserving backslash escapes. The opening quote has already been consumed.
func ScanRegexLiteral(r interface{ Get() rune }) (string, bool) {
	var b strings.Builder
	ch := r.Get()
	for ch != '"' {
		if ch == EOF {
			return "", false
		}
		b.WriteRune(ch)
		if ch == '\\' {
			ch = r.Get()
			if ch == EOF {
				return "", false
			}
			b.WriteRune(ch)
		}
		ch = r.Get()
	}
	return b.String(), true
}
