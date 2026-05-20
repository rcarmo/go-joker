package string

import "strconv"

// Split splits a string on a byte separator.
func Split(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

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

// IntRangeLabel renders a human-readable inclusive integer arity range.
func IntRangeLabel(min, max int) string {
	if min == max {
		return strconv.Itoa(min)
	}
	if min+1 == max {
		return strconv.Itoa(min) + " or " + strconv.Itoa(max)
	}
	if min+2 == max {
		return strconv.Itoa(min) + ", " + strconv.Itoa(min+1) + ", or " + strconv.Itoa(max)
	}
	if max >= 999 {
		return "at least " + strconv.Itoa(min)
	}
	return "between " + strconv.Itoa(min) + " and " + strconv.Itoa(max) + ", inclusive"
}

func parseVersionPart(part string) int64 {
	value, err := strconv.ParseInt(part, 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// ParseVersionTriplet parses a dotted x.y.z version string and returns the
// numeric major, minor, and incremental parts. A leading "v" is ignored.
func ParseVersionTriplet(version string) (major, minor, incremental int64) {
	if len(version) > 0 && version[0] == 'v' {
		version = version[1:]
	}
	parts := Split(version, '.')
	if len(parts) > 0 {
		major = parseVersionPart(parts[0])
	}
	if len(parts) > 1 {
		minor = parseVersionPart(parts[1])
	}
	if len(parts) > 2 {
		incremental = parseVersionPart(parts[2])
	}
	return
}
