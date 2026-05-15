package reader

import "unicode"

const (
	ValidCoreReason    = "not a Letter nor (Decimal) Digit (category L nor Nd)"
	ValidSymbolReason  = "not a Letter, (Decimal) Digit, nor Symbol (category L, Nd, nor S)"
	ValidVisibleReason = "not a Letter, (Decimal) Digit, Symbol, Punctuation, nor Mark (category L, Nd, S, P, nor M)"
	ValidUnicodeReason = "not a Unicode character"
	ValidASCIIReason   = "not a (7-bit) ASCII character"
	ValidAnyReason     = "not anything!?"
)

// IsCoreIdentRune reports whether r is inherently allowed in core identifiers.
func IsCoreIdentRune(r rune) bool {
	switch r {
	case '*', '+', '!', '-', '?', '=', '<', '>', '&', '_', '.', '\'', '#', '$', ':', '%':
		return true
	}
	return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9')
}

func IsValidCoreRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func IsValidSymbolRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSymbol(r)
}

func IsValidVisibleRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSymbol(r) || unicode.IsPunct(r) || unicode.IsMark(r)
}

func IsValidUnicodeRune(r rune) bool {
	return r >= 0 && r <= unicode.MaxRune
}

func IsValidASCIIRune(r rune) bool {
	return r <= unicode.MaxASCII
}

func IsValidAnyRune(r rune) bool {
	return true
}
