package string

import "bytes"

func EscapeRune(r rune) string {
	switch r {
	case ' ':
		return "\\space"
	case '\n':
		return "\\newline"
	case '\t':
		return "\\tab"
	case '\r':
		return "\\return"
	case '\b':
		return "\\backspace"
	case '\f':
		return "\\formfeed"
	default:
		return "\\" + string(r)
	}
}

func EscapeString(str string) string {
	var b bytes.Buffer
	b.WriteRune('"')
	for _, r := range str {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\t':
			b.WriteString("\\t")
		case '\r':
			b.WriteString("\\r")
		case '\n':
			b.WriteString("\\n")
		case '\f':
			b.WriteString("\\f")
		case '\b':
			b.WriteString("\\b")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteRune('"')
	return b.String()
}
