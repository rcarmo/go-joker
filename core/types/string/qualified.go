package string

import stringsdk "strings"

// SplitQualified splits a Joker qualified name of the form ns/name.
// It returns ok=false when the name is unqualified or the special single slash.
func SplitQualified(name string) (ns, local string, ok bool) {
	index := stringsdk.IndexRune(name, '/')
	if index == -1 || name == "/" {
		return "", name, false
	}
	return name[:index], name[index+1:], true
}
