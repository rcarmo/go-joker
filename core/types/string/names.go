package string

import (
	"fmt"
	stringsdk "strings"
)

// Compare returns lexical ordering for two strings.
func Compare(a, b string) int { return stringsdk.Compare(a, b) }

// TrimVarQuotePrefix removes Joker's #' var-quote prefix when present.
func TrimVarQuotePrefix(name string) string { return stringsdk.TrimPrefix(name, "#'") }

// HasJokerNamespacePrefix reports whether a namespace name is under joker.*.
func HasJokerNamespacePrefix(name string) bool { return stringsdk.HasPrefix(name, "joker.") }

// IsJokerdPath reports whether a path points inside Joker's .jokerd area.
func IsJokerdPath(path string) bool { return stringsdk.Contains(path, ".jokerd") }

// IsIgnorableBindingName reports whether a local binding name should be ignored
// by unused-binding warnings.
func IsIgnorableBindingName(name string) bool {
	return stringsdk.HasPrefix(name, "_") || stringsdk.HasPrefix(name, "&form") || stringsdk.HasPrefix(name, "&env")
}

// HasNamespaceSeparator reports whether a symbol-like name contains a slash or
// dotted namespace separator rune.
func HasNamespaceSeparator(name string, sep rune) bool { return stringsdk.ContainsRune(name, sep) }

// SplitQualified splits a Joker qualified name of the form ns/name.
// It returns ok=false when the name is unqualified or the special single slash.
func SplitQualified(name string) (ns, local string, ok bool) {
	index := stringsdk.IndexRune(name, '/')
	if index == -1 || name == "/" {
		return "", name, false
	}
	return name[:index], name[index+1:], true
}

var goNameTranslations = [][2]string{{"?", "Q"}, {"!", "BANG"}, {"<=", "LE"}, {">=", "GE"}, {"<", "LT"}, {">", "GT"}, {"=", "EQ"}, {"'", "APOS"}, {"+", "PLUS"}, {"-", "DASH"}, {"*", "STAR"}, {"/", "SLASH"}, {"&", "AMP"}, {"#", "HASH"}, {".", "DOT"}, {"%", "PCT"}, {":", "COLON"}}

// GoName converts a Joker symbol-ish string into a Go-safe identifier fragment.
func GoName(name string) string {
	for _, t := range goNameTranslations {
		name = stringsdk.ReplaceAll(name, t[0], "_"+t[1]+"_")
	}
	return name
}

// FilenameAndWhetherBracketed unwraps a <name> source identifier when present.
func FilenameAndWhetherBracketed(name string) (filename string, bracketed bool) {
	if len(name) > 1 && name[0] == '<' && name[len(name)-1] == '>' {
		return name[1 : len(name)-1], true
	}
	return name, false
}

// FilenameUnbracketed removes enclosing angle brackets from a source name.
func FilenameUnbracketed(name string) string {
	filename, _ := FilenameAndWhetherBracketed(name)
	return filename
}

// CoreNamespaceName returns the unwrapped namespace name from a bracketed core source id.
func CoreNamespaceName(name string) string {
	if filename, bracketed := FilenameAndWhetherBracketed(name); bracketed {
		return filename
	}
	panic(fmt.Sprintf("Invalid syntax for core source file namespace id: `%s'", name))
}
