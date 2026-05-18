package string

import "strconv"

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
