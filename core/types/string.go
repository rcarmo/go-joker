package types

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/rcarmo/go-joker/core/hashutil"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

type String struct {
	InfoHolder
	S string
}

var StringIndexError func(string) any

func MakeString(s string) String { return String{S: s} }

func (s String) ToString(escape bool) string {
	if escape {
		return corestr.EscapeString(s.S)
	}
	return s.S
}

func (s String) Format(w io.Writer, indent int) int {
	fmt.Fprint(w, "\"", s.S, "\"")
	return indent + utf8.RuneCountInString(s.S) + 2
}

func (s String) Equals(other interface{}) bool {
	switch other := other.(type) {
	case String:
		return s.S == other.S
	default:
		return false
	}
}

func (s String) GetType() *Type      { return RuntimeTypes.String }
func (s String) Native() interface{} { return s.S }
func (s String) Hash() uint32        { h := hashutil.New32(); h.Write([]byte(s.S)); return h.Sum32() }
func (s String) Count() int {
	if corestr.IsASCII(s.S) {
		return len(s.S)
	}
	return utf8.RuneCountInString(s.S)
}

func (s String) Nth(i int) Object {
	if i < 0 {
		panicStringIndex(fmt.Sprintf("Negative index: %d", i))
	}
	if r, n, ok := corestr.NthRune(s.S, i); ok {
		return Char{Ch: r}
	} else {
		panicStringIndex(fmt.Sprintf("Index %d exceeds string's length %d", i, n))
		return nil
	}
}

func (s String) TryNth(i int, d Object) Object {
	if i < 0 {
		return d
	}
	if r, _, ok := corestr.NthRune(s.S, i); ok {
		return Char{Ch: r}
	}
	return d
}

func (s String) Compare(other Object) int { s2 := other.(String); return corestr.Compare(s.S, s2.S) }

func StringIsASCII(s string) bool { return corestr.IsASCII(s) }

func panicStringIndex(msg string) {
	if StringIndexError != nil {
		panic(StringIndexError(msg))
	}
	panic(msg)
}
