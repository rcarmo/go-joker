package core

import (
	"fmt"
	"io"
	"math/big"
	"os"
	"unicode/utf8"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

var exitCallbacks []func()

func ExitJoker(rc int) {
	for _, f := range exitCallbacks {
		f()
	}
	os.Exit(rc)
}

func OnExit(f func()) {
	exitCallbacks = append(exitCallbacks, f)
}

func writeIndent(w io.Writer, n int) {
	space := []byte(" ")
	for i := 0; i < n; i++ {
		w.Write(space)
	}
}

func pprintObject(obj coretypes.Object, indent int, w io.Writer) int {
	switch obj := obj.(type) {
	case coretypes.Pprinter:
		return obj.Pprint(w, indent)
	default:
		s := obj.ToString(true)
		fmt.Fprint(w, s)
		return indent + len(s)
	}
}

func formatObject(obj coretypes.Object, indent int, w io.Writer) int {
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

func isComment(obj coretypes.Object) bool {
	if _, ok := obj.(coretypes.Comment); ok {
		return true
	}
	info := obj.GetInfo()
	if info == nil {
		return false
	}
	return info.Prefix == "^" || info.Prefix == "#^" || info.Prefix == "#_"
}

func isComma(obj coretypes.Object) bool {
	if c, ok := obj.(coretypes.Comment); ok && c.C == "," {
		return true
	}
	return false
}

func maybeNewLine(w io.Writer, obj, nextObj coretypes.Object, baseIndent, currentIndent int) int {
	if writeNewLines(w, obj, nextObj) > 0 {
		writeIndent(w, baseIndent)
		return baseIndent
	}
	if !isComma(nextObj) {
		fmt.Fprint(w, " ")
	}
	return currentIndent + 1
}

func FileInfoMap(name string, info os.FileInfo) coretypes.Map {
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString(name))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "size"), coretypes.IntOrBigInt(big.NewInt(info.Size())))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "mode"), coretypes.MakeInt(int(info.Mode())))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "modtime"), coretypes.MakeTime(info.ModTime()))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "dir?"), coretypes.MakeBoolean(info.IsDir()))
	return m
}

func ToBool(obj coretypes.Object) bool {
	switch obj := obj.(type) {
	case Nil:
		return false
	case coretypes.Boolean:
		return obj.B
	default:
		return true
	}
}
