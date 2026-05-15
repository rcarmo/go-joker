package reader

// IsIdentRune reports whether r may be a non-initial character in a symbol or
// keyword name.
func IsIdentRune(r rune) bool {
	switch r {
	case '"', ';', '@', '^', '`', '~', '(', ')', '[', ']', '{', '}', '\\', ',', ' ', '\t', '\n', '\r', EOF:
		// Whitespace listed above (' ', '\t', '\n', '\r') purely for speed of common cases.
		return false
	}
	return !IsJavaSpace(r)
}
