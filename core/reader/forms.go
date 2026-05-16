package reader

// ShouldAppendMapCommentSurrogate reports whether a map literal scanner should
// append a surrogate form after a comment to keep the map form count even in
// format mode. The caller owns comment detection and surrogate construction.
func ShouldAppendMapCommentSurrogate(formatMode bool, isComment bool) bool {
	return formatMode && isComment
}

// ShouldReportReadError reports whether a reader error should be emitted as a
// linter error rather than raised as a read exception.
func ShouldReportReadError(linterMode bool) bool {
	return linterMode
}

// ShouldSuppressUnreadConditionalBranch reports whether a reader conditional
// branch should be read with SUPPRESS_READ because a prior branch was selected
// or the current feature is unavailable.
func ShouldSuppressUnreadConditionalBranch(hasResult bool, featureEnabled bool) bool {
	return hasResult || !featureEnabled
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

// IsUnquoteSplice reports whether an unquote form uses ~@ splicing.
func IsUnquoteSplice(peek rune) bool {
	return peek == '@'
}

// UnquotePrefix returns the format-mode prefix for an unquote form.
func UnquotePrefix(splicing bool) string {
	if splicing {
		return "~@"
	}
	return "~"
}

// NamespacedMapPrefix returns the format-mode prefix for #:/#:: map literals.
type NamespacedMapStartKind int

const (
	NamespacedMapStartNamespace NamespacedMapStartKind = iota
	NamespacedMapStartMap
	NamespacedMapStartMissingNamespace
)

// ClassifyNamespacedMapStart reports what follows #:/#:: while reading a
// namespaced map. Root owns concrete namespace symbol reading and errors.
func ClassifyNamespacedMapStart(r rune, auto bool) NamespacedMapStartKind {
	if IsWhitespace(r) {
		if auto {
			return NamespacedMapStartMap
		}
		return NamespacedMapStartMissingNamespace
	}
	if r == '{' {
		return NamespacedMapStartMap
	}
	return NamespacedMapStartNamespace
}

func NamespacedMapPrefix(auto bool, namespace string) string {
	prefix := "#:"
	if auto {
		prefix += ":"
	}
	return prefix + namespace
}

// ReaderMacroPrefix returns the format-mode prefix for simple reader macros.
func ReaderMacroPrefix(r rune) (string, bool) {
	switch r {
	case '\'', '@', '`', '^':
		return string(r), true
	default:
		return "", false
	}
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

// OrderedArgValues returns positional function literal args, appending amp and
// rest args when the %& sentinel (-1) is present. The bool result is false for
// invalid indexes such as %0 or indexes outside the filled positional range.
func OrderedArgValues[T any](args map[int]T, amp T) ([]T, bool) {
	values := make([]T, len(args))
	positionalCount := len(args)
	if _, ok := args[-1]; ok {
		positionalCount--
	}
	for key, value := range args {
		if key == -1 {
			continue
		}
		if key <= 0 || key > positionalCount {
			return nil, false
		}
		values[key-1] = value
	}
	if rest, ok := args[-1]; ok {
		values[len(args)-1] = amp
		values = append(values, rest)
	}
	return values, true
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

// IsTopLevelSpliceSurrogate reports whether a multi-read surrogate should be
// rejected at top level. Root keeps ObjectInfo ownership and passes only the
// presence bit to avoid importing root core here.
func IsTopLevelSpliceSurrogate(hasInfo bool) bool {
	return hasInfo
}
