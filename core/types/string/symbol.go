package string

import stringsdk "strings"

// IsInteropName reports whether a symbol name uses Joker's Java interop naming
// conventions such as a leading dot, trailing dot, or embedded dollar sign.
func IsInteropName(name string) bool {
	return stringsdk.HasPrefix(name, ".") || stringsdk.HasSuffix(name, ".") || stringsdk.Contains(name, "$")
}

// IsRecordConstructorName reports whether a symbol name denotes a record
// constructor helper.
func IsRecordConstructorName(name string) bool {
	return stringsdk.HasPrefix(name, "->") || stringsdk.HasPrefix(name, "map->")
}
