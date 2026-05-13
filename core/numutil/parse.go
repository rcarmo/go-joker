package numutil

import "strconv"

// ParseInt wraps strconv.ParseInt for shared numeric lexical parsing.
func ParseInt(s string, base, bitSize int) (int64, error) {
	return strconv.ParseInt(s, base, bitSize)
}

// ParseFloat64 wraps strconv.ParseFloat for shared float lexical parsing.
func ParseFloat64(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
