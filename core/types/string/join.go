package string

import stringsdk "strings"

// JoinDotted joins path/name parts using a dot separator.
func JoinDotted(parts []string) string {
	return stringsdk.Join(parts, ".")
}
