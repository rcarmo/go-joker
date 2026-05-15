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
