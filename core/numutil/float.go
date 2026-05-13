package numutil

import (
	"math"
	stringsdk "strings"
)

// ComputeFloatPrecision estimates the minimum useful bit precision for parsing
// a numeric literal as a big float.
func ComputeFloatPrecision(s string) uint {
	prec := 53.0 // Default to precision for float64
	if s == "" {
		return uint(prec)
	}
	if s[0] == '-' || s[0] == '+' {
		s = s[1:]
	}
	if s == "Inf" || s == "inf" || s == "NaN" {
		return uint(prec)
	}

	bitsNeeded := 0.0
	bitsPerDigit := 3.33 // log2(10)
	exponentUpper, exponentLower := 'E', 'e'

	if len(s) > 2 && s[0] == '0' && stringsdk.ContainsAny(s[1:2], "bBoOxX") {
		switch s[1] {
		case 'b', 'B':
			bitsPerDigit = 1
		case 'o', 'O':
			bitsPerDigit = 3
		case 'x', 'X':
			bitsPerDigit = 4
		default:
			panic("internal error examining numeric literal")
		}
		exponentUpper, exponentLower = 'P', 'p'
		s = s[2:]
	}

	for _, c := range s {
		if c == exponentUpper || c == exponentLower {
			break
		}
		if ('0' <= c && c <= '9') || ('A' <= c && c <= 'F') || ('a' <= c && c <= 'f') {
			bitsNeeded += bitsPerDigit
		}
	}

	bitsNeeded = math.Max(prec, math.Ceil(bitsNeeded))
	return uint(bitsNeeded)
}

// NeedsDecimalSuffix reports whether a %g-formatted float already includes a
// decimal or exponent marker.
func NeedsDecimalSuffix(rendered string) bool {
	return !stringsdk.ContainsAny(rendered, ".e")
}
