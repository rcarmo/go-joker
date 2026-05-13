package string

import stringsdk "strings"

// Compare returns lexical ordering for two strings.
func Compare(a, b string) int {
	return stringsdk.Compare(a, b)
}
