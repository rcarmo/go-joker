package core

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"
	"math/big"
	"os"
	"unicode/utf8"
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

func pprintObject(obj Object, indent int, w io.Writer) int {
	switch obj := obj.(type) {
	case coretypes.Pprinter:
		return obj.Pprint(w, indent)
	default:
		s := obj.ToString(true)
		fmt.Fprint(w, s)
		return indent + len(s)
	}
}

func formatObject(obj Object, indent int, w io.Writer) int {
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

func isComment(obj Object) bool {
	if _, ok := obj.(coretypes.Comment); ok {
		return true
	}
	info := obj.GetInfo()
	if info == nil {
		return false
	}
	return info.Prefix == "^" || info.Prefix == "#^" || info.Prefix == "#_"
}

func isComma(obj Object) bool {
	if c, ok := obj.(coretypes.Comment); ok && c.C == "," {
		return true
	}
	return false
}

func maybeNewLine(w io.Writer, obj, nextObj Object, baseIndent, currentIndent int) int {
	if writeNewLines(w, obj, nextObj) > 0 {
		writeIndent(w, baseIndent)
		return baseIndent
	}
	if !isComma(nextObj) {
		fmt.Fprint(w, " ")
	}
	return currentIndent + 1
}

func FileInfoMap(name string, info os.FileInfo) Map {
	m := collectionConstruction.NewEmptyArrayMap()
	m.Add(MakeKeyword("name"), MakeString(name))
	m.Add(MakeKeyword("size"), coretypes.IntOrBigInt(big.NewInt(info.Size())))
	m.Add(MakeKeyword("mode"), coretypes.MakeInt(int(info.Mode())))
	m.Add(MakeKeyword("modtime"), coretypes.MakeTime(info.ModTime()))
	m.Add(MakeKeyword("dir?"), coretypes.MakeBoolean(info.IsDir()))
	return m
}

func ToBool(obj Object) bool {
	switch obj := obj.(type) {
	case Nil:
		return false
	case coretypes.Boolean:
		return obj.B
	default:
		return true
	}
}
