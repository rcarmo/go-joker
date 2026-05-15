package reader

import (
	"fmt"
	"strings"
)

type IdentTokenError string

func (e IdentTokenError) Error() string { return string(e) }

type RuneGetterUngetter interface {
	Get() rune
	Unget()
}

// ScanIdentToken scans an identifier token after first has already been read.
func ScanIdentToken(r RuneGetterUngetter, first rune) (string, rune, error) {
	var b strings.Builder
	if first != ':' {
		b.WriteRune(first)
	}
	var lastAdded rune
	ch := r.Get()
	for IsIdentRune(ch) {
		if ch == ':' && lastAdded == ':' {
			return "", lastAdded, IdentTokenError("Invalid use of ':' in symbol name")
		}
		b.WriteRune(ch)
		lastAdded = ch
		ch = r.Get()
	}
	r.Unget()
	return b.String(), lastAdded, nil
}

// ValidateIdentToken validates reader identifier token edge cases that do not
// require root object construction or namespace resolution.
func ValidateIdentToken(first rune, token string, lastAdded rune) error {
	if lastAdded == ':' || (lastAdded == '/' && len(token) > 1) {
		return IdentTokenError(fmt.Sprintf("Invalid use of %c in symbol name", lastAdded))
	}
	if token == "" {
		return IdentTokenError("Invalid keyword: :")
	}
	if first == ':' && token[0] == '/' && len(token) > 1 {
		return IdentTokenError("Blank namespaces are not allowed")
	}
	return nil
}
