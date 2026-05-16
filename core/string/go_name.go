package string

import (
	"fmt"
	stringsdk "strings"
)

var goNameTranslations = [][2]string{
	{"?", "Q"},
	{"!", "BANG"},
	{"<=", "LE"},
	{">=", "GE"},
	{"<", "LT"},
	{">", "GT"},
	{"=", "EQ"},
	{"'", "APOS"},
	{"+", "PLUS"},
	{"-", "DASH"},
	{"*", "STAR"},
	{"/", "SLASH"},
	{"&", "AMP"},
	{"#", "HASH"},
	{".", "DOT"},
	{"%", "PCT"},
	{":", "COLON"},
}

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
