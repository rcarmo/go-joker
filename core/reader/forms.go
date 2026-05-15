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

// IsConditionalSplice reports whether a reader conditional uses #?@ splicing.
func IsConditionalSplice(peek rune) bool {
	return peek == '@'
}

// ConditionalPrefix returns the format-mode prefix for a reader conditional.
func ConditionalPrefix(splicing bool) string {
	if splicing {
		return "#?@"
	}
	return "#?"
}

// FillMissingArgIndexes fills missing positive argument indexes from 1 through
// max-1. The caller supplies fresh values to preserve root symbol generation.
func FillMissingArgIndexes[T any](args map[int]T, makeValue func() T) {
	max := 0
	for k := range args {
		if k > max {
			max = k
		}
	}
	for i := 1; i < max; i++ {
		if _, ok := args[i]; !ok {
			args[i] = makeValue()
		}
	}
}

// PopLastForm removes and returns the last pending form. The bool result is
// false when forms is empty.
func PopLastForm[T any](forms []T) (T, []T, bool) {
	var zero T
	if len(forms) == 0 {
		return zero, forms, false
	}
	last := forms[len(forms)-1]
	return last, forms[:len(forms)-1], true
}
