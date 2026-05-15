package reader

import "unicode"

type StringEscapeKind int

const (
	StringEscapeSimple StringEscapeKind = iota
	StringEscapeUnicode
	StringEscapeOctal
	StringEscapeUnsupported
)

// ClassifyStringEscape classifies a string escape rune without consuming any
// following digits. Format-mode preservation remains a caller concern.
func ClassifyStringEscape(esc rune) StringEscapeKind {
	if _, ok := DecodeSimpleStringEscape(esc); ok {
		return StringEscapeSimple
	}
	if esc == 'u' {
		return StringEscapeUnicode
	}
	if unicode.IsDigit(esc) {
		return StringEscapeOctal
	}
	return StringEscapeUnsupported
}

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
