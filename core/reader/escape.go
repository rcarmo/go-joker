package reader

// DecodeSimpleStringEscape decodes the non-unicode, non-octal string escape
// runes recognized by the reader. The bool result reports whether esc was a
// supported simple escape.
func DecodeSimpleStringEscape(esc rune) (rune, bool) {
	switch esc {
	case '\\':
		return '\\', true
	case '"':
		return '"', true
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'r':
		return '\r', true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	default:
		return 0, false
	}
}
