package reader

import "fmt"

type IdentTokenError string

func (e IdentTokenError) Error() string { return string(e) }

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
