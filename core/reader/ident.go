package reader

// IsStandaloneSlashSymbol reports whether / is followed by a delimiter and
// should be read as the core slash symbol rather than as an identifier token.
func IsStandaloneSlashSymbol(r rune, peek rune) bool {
	return r == '/' && IsDelimiter(peek)
}

// IsKeywordIdent reports whether a scanned identifier token started as a keyword.
func IsKeywordIdent(first rune) bool {
	return first == ':'
}

// IsAutoResolvedKeywordIdent reports whether a keyword token is the reader's
// auto-resolved ::keyword form. The caller owns namespace resolution.
func IsAutoResolvedKeywordIdent(first rune, token string) bool {
	return IsKeywordIdent(first) && len(token) > 0 && token[0] == ':'
}

// IdentLiteralKind classifies identifier tokens that are reader literals rather
// than symbols. The root reader keeps ownership of concrete Object construction.
type IdentLiteralKind int

const (
	IdentLiteralSymbol IdentLiteralKind = iota
	IdentLiteralNil
	IdentLiteralTrue
	IdentLiteralFalse
)

// ClassifyIdentLiteral reports whether token is a special literal identifier.
func ClassifyIdentLiteral(token string) IdentLiteralKind {
	switch token {
	case "nil":
		return IdentLiteralNil
	case "true":
		return IdentLiteralTrue
	case "false":
		return IdentLiteralFalse
	default:
		return IdentLiteralSymbol
	}
}

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
