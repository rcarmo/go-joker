package reader

import "strings"

type RunePeekerGetter interface {
	Peek() rune
	Get() rune
}

// ReadCommentText consumes and returns comment text through, but not including,
// newline or EOF. The initial comment marker should still be unread/peekable.
func ReadCommentText(r RunePeekerGetter) string {
	var b strings.Builder
	ch := r.Peek()
	for ch != '\n' && ch != EOF {
		b.WriteRune(ch)
		r.Get()
		ch = r.Peek()
	}
	return b.String()
}

// SkipLine consumes through newline or EOF and returns the next rune after the
// line terminator, matching root whitespace skipping behavior.
func SkipLine(r interface{ Get() rune }, first rune) rune {
	ch := first
	for ch != '\n' && ch != EOF {
		ch = r.Get()
	}
	return r.Get()
}
