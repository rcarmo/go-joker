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
