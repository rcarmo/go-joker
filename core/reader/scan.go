package reader

import "strings"

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
