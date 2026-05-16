package reader

// TopLevelTriviaKind classifies format-preserved reader trivia that is handled
// before normal form dispatch. Root keeps concrete Comment construction.
type TopLevelTriviaKind int

const (
	TopLevelTriviaNone TopLevelTriviaKind = iota
	TopLevelTriviaComma
	TopLevelTriviaComment
)

// ClassifyTopLevelTrivia reports whether the current rune starts reader trivia
// that should be returned as a form in format mode. Whitespace/comment skipping
// is handled separately before this point.
func ClassifyTopLevelTrivia(r rune, peek rune) TopLevelTriviaKind {
	switch {
	case r == ',':
		return TopLevelTriviaComma
	case r == ';' || r == '#' && IsCommentStart(r, peek):
		return TopLevelTriviaComment
	default:
		return TopLevelTriviaNone
	}
}
