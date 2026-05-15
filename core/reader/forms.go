package reader

// ShouldAppendMapCommentSurrogate reports whether a map literal scanner should
// append a surrogate form after a comment to keep the map form count even in
// format mode. The caller owns comment detection and surrogate construction.
func ShouldAppendMapCommentSurrogate(formatMode bool, isComment bool) bool {
	return formatMode && isComment
}

// HasEvenFormCount reports whether a delimited form sequence contains an even
// number of forms.
func HasEvenFormCount(count int) bool {
	return count%2 == 0
}

// IsBareArgLiteral reports whether a % reader literal has no following arg
// designator and should default to %1.
func IsBareArgLiteral(peek rune) bool {
	return IsWhitespace(peek) || IsTerminatingMacro(peek)
}

// ContinueDelimitedForms reports whether a delimited form reader should keep
// reading because the closing delimiter has not been reached or because there
// are pending spliced forms to drain.
func ContinueDelimitedForms(peek rune, closing rune, pendingForms int) bool {
	return peek != closing || pendingForms != 0
}

// NeedsConditionalPair reports whether reader conditional parsing hit a closing
// delimiter immediately after a feature, which means the conditional has an odd
// number of forms.
func NeedsConditionalPair(pendingForms int, peek rune, closing rune) bool {
	return pendingForms == 0 && peek == closing
}
