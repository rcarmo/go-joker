package reader

import (
	"errors"
	"unicode"

	"github.com/rcarmo/go-joker/core/numutil"
)

type NumberTokenKind int

const (
	NumberTokenInt NumberTokenKind = iota
	NumberTokenFloat
	NumberTokenRatio
	NumberTokenBigInt
	NumberTokenBigFloat
)

type NumberToken struct {
	Kind     NumberTokenKind
	Original string
	Digits   string
	Base     int
}

// AnalyzeNumberToken classifies a reader number token and returns the string/base
// to pass to the concrete numeric parser. Root core keeps Object construction.
func AnalyzeNumberToken(str string) (NumberToken, error) {
	if str == "" {
		return NumberToken{}, errors.New("empty number token")
	}
	isDouble, isHex, isExp, isRatio, baseLen, nonDigits := false, false, false, false, 0, 0
	var last rune
	for i, r := range str {
		switch r {
		case '.':
			isDouble = true
		case '/':
			isRatio = true
		case 'x', 'X':
			isHex = true
		case 'e', 'E':
			isExp = true
		case 'r', 'R':
			if baseLen == 0 {
				baseLen = i
			}
		}
		if !unicode.IsDigit(r) {
			nonDigits++
		}
		last = r
	}
	if baseLen != 0 {
		baseInt, err := numutil.ParseInt(str[0:baseLen], 0, 0)
		if err != nil {
			return NumberToken{}, err
		}
		negative := false
		if baseInt < 0 {
			baseInt = -baseInt
			negative = true
		}
		if baseInt < 2 || baseInt > 36 {
			return NumberToken{}, errors.New("invalid radix")
		}
		number := str[baseLen+1:]
		if negative {
			number = "-" + number
		}
		return NumberToken{Kind: NumberTokenInt, Original: str, Digits: number, Base: int(baseInt)}, nil
	}
	if isRatio {
		if nonDigits > 2 || nonDigits > 1 && str[0] != '-' && str[0] != '+' {
			return NumberToken{}, errors.New("invalid ratio")
		}
		return NumberToken{Kind: NumberTokenRatio, Original: str, Digits: str}, nil
	}
	if last == 'N' {
		if len(str) == 1 {
			return NumberToken{}, errors.New("empty bigint token")
		}
		return NumberToken{Kind: NumberTokenBigInt, Original: str, Digits: str[:len(str)-1]}, nil
	}
	if last == 'M' {
		if len(str) == 1 {
			return NumberToken{}, errors.New("empty bigfloat token")
		}
		return NumberToken{Kind: NumberTokenBigFloat, Original: str, Digits: str[:len(str)-1]}, nil
	}
	if isDouble || (!isHex && isExp) {
		return NumberToken{Kind: NumberTokenFloat, Original: str, Digits: str}, nil
	}
	return NumberToken{Kind: NumberTokenInt, Original: str, Digits: str}, nil
}
