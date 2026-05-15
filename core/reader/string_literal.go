package reader

import (
	"fmt"
	"strings"
)

// UnicodeEscapeDecoder decodes unicode/octal string escapes. The caller owns
// exact error wording and reader position semantics.
type UnicodeEscapeDecoder func(initial rune, length, base int, exactLength bool) rune

// ScanStringLiteral consumes a string literal body through the closing quote.
// The opening quote has already been consumed. Format mode preserves the
// backslash and following rune, matching root reader/printer behavior.
func ScanStringLiteral(r interface{ Get() rune }, formatMode bool, decodeUnicode UnicodeEscapeDecoder) (string, error) {
	var b strings.Builder
	ch := r.Get()
	for ch != '"' {
		if ch == '\\' {
			ch = r.Get()
			if formatMode {
				b.WriteRune('\\')
			} else {
				switch ClassifyStringEscape(ch) {
				case StringEscapeSimple:
					ch, _ = DecodeSimpleStringEscape(ch)
				case StringEscapeUnicode:
					ch = decodeUnicode(r.Get(), 4, 16, true)
				case StringEscapeOctal:
					ch = decodeUnicode(ch, 3, 8, false)
				default:
					return "", fmt.Errorf("Unsupported escape character: \\%s", string(ch))
				}
			}
		}
		if ch == EOF {
			return "", fmt.Errorf("Non-terminated string literal")
		}
		b.WriteRune(ch)
		ch = r.Get()
	}
	return b.String(), nil
}
