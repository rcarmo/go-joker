package runtime

import (
	"fmt"
	"io"
	"unicode/utf8"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func WriteIndent(w io.Writer, n int) {
	space := []byte(" ")
	for i := 0; i < n; i++ {
		w.Write(space)
	}
}

func PprintObject(obj coretypes.Object, indent int, w io.Writer) int {
	switch obj := obj.(type) {
	case coretypes.Pprinter:
		return obj.Pprint(w, indent)
	default:
		s := obj.ToString(true)
		fmt.Fprint(w, s)
		return indent + len(s)
	}
}

func FormatObject(obj coretypes.Object, indent int, w io.Writer) int {
	if info := obj.GetInfo(); info != nil {
		fmt.Fprint(w, info.Prefix)
		indent += utf8.RuneCountInString(info.Prefix)
	}
	switch obj := obj.(type) {
	case coretypes.Formatter:
		return obj.Format(w, indent)
	default:
		s := obj.ToString(true)
		fmt.Fprint(w, s)
		return indent + utf8.RuneCountInString(s)
	}
}

func IsComment(obj coretypes.Object) bool {
	if _, ok := obj.(coretypes.Comment); ok {
		return true
	}
	info := obj.GetInfo()
	if info == nil {
		return false
	}
	return info.Prefix == "^" || info.Prefix == "#^" || info.Prefix == "#_"
}

func IsComma(obj coretypes.Object) bool {
	if c, ok := obj.(coretypes.Comment); ok && c.C == "," {
		return true
	}
	return false
}

func MaybeNewLine(w io.Writer, obj, nextObj coretypes.Object, baseIndent, currentIndent int) int {
	if WriteNewLines(w, obj, nextObj) > 0 {
		WriteIndent(w, baseIndent)
		return baseIndent
	}
	if !IsComma(nextObj) {
		fmt.Fprint(w, " ")
	}
	return currentIndent + 1
}

func WriteNewLines(w io.Writer, prevObj coretypes.Object, obj coretypes.Object) int {
	cnt := NewLineCount(prevObj, obj)
	for i := 0; i < cnt; i++ {
		fmt.Fprint(w, "\n")
	}
	return cnt
}

func NewLineCount(obj, nextObj coretypes.Object) int {
	info, nextInfo := obj.GetInfo(), nextObj.GetInfo()
	if info == nil || nextInfo == nil {
		return 0
	}
	return nextInfo.StartLine - info.EndLine
}
