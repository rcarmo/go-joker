package reader

import "unicode"

type InitialTokenKind int

const (
	InitialTokenIdent InitialTokenKind = iota
	InitialTokenNumber
)

// ClassifyInitialToken classifies root-independent reader dispatch for tokens
// that can begin either identifiers or numbers. Dialect-specific behavior is
// represented by allowLeadingDotNumber.
func ClassifyInitialToken(r rune, peek rune, allowLeadingDotNumber bool) InitialTokenKind {
	switch {
	case unicode.IsDigit(r):
		return InitialTokenNumber
	case r == '.' && allowLeadingDotNumber && unicode.IsDigit(peek):
		return InitialTokenNumber
	case (r == '-' || r == '+') && unicode.IsDigit(peek):
		return InitialTokenNumber
	default:
		return InitialTokenIdent
	}
}
