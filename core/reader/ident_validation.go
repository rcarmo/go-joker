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

// IdentValidationReason returns the explanatory reason for a rune that failed
// the configured set/range validation predicates. Empty string means the rune
// passed both predicates.
func IdentValidationReason(r rune, setOK bool, setWhy string, rangeOK bool, rangeWhy string) string {
	if setOK && rangeOK {
		return ""
	}
	if setOK {
		return rangeWhy
	}
	if rangeOK {
		return setWhy
	}
	return setWhy + "; " + rangeWhy
}

type IdentValidationIssue struct {
	Rune   rune
	Index  int
	Reason string
}

// FindIdentValidationIssues returns validation issues for lint-time identifier
// text. Nil input has no issues because root Symbol/Keyword namespaces can be nil.
func FindIdentValidationIssues(s *string, setFn func(rune) bool, setWhy string, rangeFn func(rune) bool, rangeWhy string) []IdentValidationIssue {
	if s == nil {
		return nil
	}
	var issues []IdentValidationIssue
	k := 0
	for _, r := range *s {
		setOK := setFn(r)
		rangeOK := rangeFn(r)
		if !IsCoreIdentRune(r) && (!setOK || !rangeOK) {
			issues = append(issues, IdentValidationIssue{
				Rune:   r,
				Index:  k,
				Reason: IdentValidationReason(r, setOK, setWhy, rangeOK, rangeWhy),
			})
		}
		k++
	}
	return issues
}
