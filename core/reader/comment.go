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
