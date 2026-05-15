package reader

import "unicode"

const EOF = -1

func IsJavaSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r': // Listed here purely for speed of common cases.
		return true
	case 0xa0 /*&nbsp;*/, 0x85 /*NEL*/, 0x2007 /*&numsp;*/, 0x202f /*narrow non-break space*/ :
		return false
	case 0x1c /*FS*/, 0x1d /*GS*/, 0x1e /*RS*/, 0x1f /*US*/ :
		return true
	default:
		if r > unicode.MaxLatin1 && unicode.In(r, unicode.Zl, unicode.Zp, unicode.Zs) {
			return true
		}
	}
	return unicode.IsSpace(r)
}

func IsWhitespace(r rune) bool {
	return IsJavaSpace(r) || r == ','
}

func IsDelimiter(r rune) bool {
	return IsWhitespace(r) || IsTerminatingMacro(r) || r == EOF
}

func IsTerminatingMacro(r rune) bool {
	switch r {
	case '"', ';', '@', '^', '`', '~', '(', ')', '[', ']', '{', '}', '\\':
		return true
	}
	return false
}
