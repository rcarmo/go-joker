package reader

import "strings"

// ConsumeExpected consumes exactly expected from r. It returns the unexpected
// rune and false on mismatch.
func ConsumeExpected(r interface{ Get() rune }, expected string) (rune, bool) {
	for _, want := range expected {
		got := r.Get()
		if got != want {
			return got, false
		}
	}
	return 0, true
}

// PeekDelimiter reports whether r's next rune is a reader delimiter.
func PeekDelimiter(r interface{ Peek() rune }) bool {
	return IsDelimiter(r.Peek())
}

// ScanUntilDelimiter consumes runes until a reader delimiter or EOF is seen,
// then pushes that delimiter/EOF sentinel back through Unget to preserve root
// reader behavior.
func ScanUntilDelimiter(r RuneGetterUngetter) string {
	var b strings.Builder
	ch := r.Get()
	for !IsDelimiter(ch) {
		b.WriteRune(ch)
		ch = r.Get()
	}
	r.Unget()
	return b.String()
}
