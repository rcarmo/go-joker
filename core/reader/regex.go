package reader

import "strings"

// InvalidRegexAction describes how the root reader should handle a regex body
// that Go's regexp compiler rejects. Root keeps Object/error construction.
type InvalidRegexAction int

const (
	InvalidRegexError InvalidRegexAction = iota
	InvalidRegexPlaceholder
	InvalidRegexPreserveString
)

// ClassifyInvalidRegexAction returns the root action for an invalid regex under
// current reader modes. Linter mode wins over format mode, matching root reader
// behavior before extraction.
func ClassifyInvalidRegexAction(linterMode bool, formatMode bool) InvalidRegexAction {
	switch {
	case linterMode:
		return InvalidRegexPlaceholder
	case formatMode:
		return InvalidRegexPreserveString
	default:
		return InvalidRegexError
	}
}

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
