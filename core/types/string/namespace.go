package string

import stringsdk "strings"

// IsJokerdPath reports whether a path points inside Joker's .jokerd area.
func IsJokerdPath(path string) bool {
	return stringsdk.Contains(path, ".jokerd")
}
