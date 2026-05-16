package reader

import "unicode"

// ReadFormKind classifies the first rune of a top-level form after whitespace
// and format trivia have been handled. Root keeps concrete Object construction
// and reader-position side effects such as Unget/popPos.
type ReadFormKind int

const (
	ReadFormIdent ReadFormKind = iota
	ReadFormCharacter
	ReadFormNumber
	ReadFormArgSymbol
	ReadFormString
	ReadFormList
	ReadFormVector
	ReadFormMap
	ReadFormStandaloneSlash
	ReadFormQuote
	ReadFormDeref
	ReadFormUnquote
	ReadFormSyntaxQuote
	ReadFormMeta
	ReadFormDispatch
	ReadFormEOF
	ReadFormClosingDelimiter
)

// ClassifyReadForm classifies the current rune for Read dispatch. The caller
// supplies peek only for current runes that need lookahead (., +, -, /); passing
// zero for other runes avoids unnecessary reader-window mutations.
func ClassifyReadForm(r rune, peek rune, argsActive bool, formatMode bool, allowLeadingDotNumber bool) ReadFormKind {
	switch {
	case r == '\\':
		return ReadFormCharacter
	case unicode.IsDigit(r):
		return ReadFormNumber
	case r == '.' || r == '-' || r == '+':
		if ClassifyInitialToken(r, peek, allowLeadingDotNumber) == InitialTokenNumber {
			return ReadFormNumber
		}
		return ReadFormIdent
	case r == '%' && argsActive:
		if formatMode {
			return ReadFormIdent
		}
		return ReadFormArgSymbol
	case r == '"':
		return ReadFormString
	case r == '(':
		return ReadFormList
	case r == '[':
		return ReadFormVector
	case r == '{':
		return ReadFormMap
	case IsStandaloneSlashSymbol(r, peek):
		return ReadFormStandaloneSlash
	case r == '\'':
		return ReadFormQuote
	case r == '@':
		return ReadFormDeref
	case r == '~':
		return ReadFormUnquote
	case r == '`':
		return ReadFormSyntaxQuote
	case r == '^':
		return ReadFormMeta
	case r == '#':
		return ReadFormDispatch
	case r == EOF:
		return ReadFormEOF
	case IsClosingDelimiter(r):
		return ReadFormClosingDelimiter
	default:
		return ReadFormIdent
	}
}

// NeedsReadFormPeek reports whether ClassifyReadForm needs caller-provided
// lookahead for this rune.
func NeedsReadFormPeek(r rune) bool {
	switch r {
	case '.', '-', '+', '/':
		return true
	default:
		return false
	}
}
