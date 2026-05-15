package reader

// NamedCharacter maps reader character literal prefixes to their remaining
// token text and resulting rune. The first rune after '\\' has already been
// consumed; peek is the next rune.
func NamedCharacter(first rune, peek rune) (ending string, value rune, ok bool) {
	switch first {
	case 's':
		if peek == 'p' {
			return "pace", ' ', true
		}
	case 'n':
		if peek == 'e' {
			return "ewline", '\n', true
		}
	case 't':
		if peek == 'a' {
			return "ab", '\t', true
		}
	case 'f':
		if peek == 'o' {
			return "ormfeed", '\f', true
		}
	case 'b':
		if peek == 'a' {
			return "ackspace", '\b', true
		}
	case 'r':
		if peek == 'e' {
			return "eturn", '\r', true
		}
	}
	return "", 0, false
}
