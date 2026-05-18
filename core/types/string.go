package types

import (
	"fmt"
	"io"
	"sync"
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
	if StringIsASCII(s.S) {
		return len(s.S)
	}
	return utf8.RuneCountInString(s.S)
}

func (s String) Nth(i int) Object {
	if i < 0 {
		panicStringIndex(fmt.Sprintf("Negative index: %d", i))
	}
	if i < len(s.S) && StringIsASCII(s.S) {
		return Char{Ch: rune(s.S[i])}
	}
	n := 0
	for _, r := range s.S {
		if n == i {
			return Char{Ch: r}
		}
		n++
	}
	panicStringIndex(fmt.Sprintf("Index %d exceeds string's length %d", i, n))
	return nil
}

func (s String) TryNth(i int, d Object) Object {
	if i < 0 {
		return d
	}
	if i < len(s.S) && StringIsASCII(s.S) {
		return Char{Ch: rune(s.S[i])}
	}
	n := 0
	for _, r := range s.S {
		if n == i {
			return Char{Ch: r}
		}
		n++
	}
	return d
}

func (s String) Compare(other Object) int { s2 := other.(String); return corestr.Compare(s.S, s2.S) }

var asciiCache sync.Map // string -> bool

func StringIsASCII(s string) bool {
	if len(s) <= 8 {
		for i := 0; i < len(s); i++ {
			if s[i] >= 0x80 {
				return false
			}
		}
		return true
	}
	if v, ok := asciiCache.Load(s); ok {
		return v.(bool)
	}
	result := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			result = false
			break
		}
	}
	asciiCache.Store(s, result)
	return result
}

func panicStringIndex(msg string) {
	if StringIndexError != nil {
		panic(StringIndexError(msg))
	}
	panic(msg)
}
