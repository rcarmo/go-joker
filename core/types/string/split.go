package string

// SplitWhitespace splits a string on ASCII whitespace without allocating a
// regexp or depending on unicode tables.
func SplitWhitespace(s string) []string {
	var res []string
	start := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		space := c == ' ' || c == '\n' || c == '\t' || c == '\r' || c == '\v' || c == '\f'
		if space {
			if start >= 0 {
				res = append(res, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		res = append(res, s[start:])
	}
	return res
}
