package string

import stringsdk "strings"

// IsIgnorableBindingName reports whether a local binding name should be ignored
// by unused-binding warnings.
func IsIgnorableBindingName(name string) bool {
	return stringsdk.HasPrefix(name, "_") || stringsdk.HasPrefix(name, "&form") || stringsdk.HasPrefix(name, "&env")
}

// HasNamespaceSeparator reports whether a symbol-like name contains a slash or
// dotted namespace separator rune.
func HasNamespaceSeparator(name string, sep rune) bool {
	return stringsdk.ContainsRune(name, sep)
}
