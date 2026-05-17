package collections

import "strings"

// FormatDelimited renders collection delimiters around caller-provided item
// strings. Root core supplies Object formatting; this package owns only the
// root-independent delimiter/separator mechanics.
func FormatDelimited(prefix string, suffix string, sep string, iter func(func(string) bool)) string {
	var b strings.Builder
	b.WriteString(prefix)
	first := true
	iter(func(item string) bool {
		if !first {
			b.WriteString(sep)
		}
		first = false
		b.WriteString(item)
		return true
	})
	b.WriteString(suffix)
	return b.String()
}

// FormatPairDelimited is the pair-oriented companion to FormatDelimited.
func FormatPairDelimited[K comparable, V any](prefix string, suffix string, pairSep string, entrySep string, iter func(func(Pair[K, V]) bool), formatKey func(K) string, formatValue func(V) string) string {
	return FormatDelimited(prefix, suffix, entrySep, func(yield func(string) bool) {
		iter(func(p Pair[K, V]) bool {
			return yield(formatKey(p.Key) + pairSep + formatValue(p.Value))
		})
	})
}
