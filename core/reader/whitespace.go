package reader

// ShouldPreserveComma reports whether a comma should be returned as a format
// comment instead of being consumed as whitespace.
func ShouldPreserveComma(formatMode bool, r rune) bool {
	return formatMode && r == ','
}

// ShouldSkipReaderComment reports whether reader whitespace skipping should
// consume a comment/shebang line.
func ShouldSkipReaderComment(formatMode bool, r rune, peek rune) bool {
	return !formatMode && (r == ';' || (r == '#' && IsCommentStart(r, peek)))
}

// ShouldDiscardNextForm reports whether whitespace skipping should consume a
// #_ discarded form.
func ShouldDiscardNextForm(formatMode bool, r rune, peek rune) bool {
	return !formatMode && r == '#' && peek == '_'
}

// SkipWhitespaceRun consumes whitespace starting at first and returns the first
// non-whitespace rune. It does not handle comments or discard forms; callers
// that need full reader whitespace semantics should use their orchestration
// layer instead.
func SkipWhitespaceRun(r interface{ Get() rune }, first rune) rune {
	ch := first
	for IsWhitespace(ch) {
		ch = r.Get()
	}
	return ch
}
