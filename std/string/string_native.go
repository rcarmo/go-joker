package string

import (
	"bytes"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	. "github.com/rcarmo/go-joker/core"
)

var newLine *regexp.Regexp

func padRight(s, pad string, n int) string {
	toAdd := n - utf8.RuneCountInString(s)
	if toAdd <= 0 {
		return s
	}
	c := utf8.RuneCountInString(pad)
	if c == 0 {
		return s
	}
	d := toAdd / c
	r := toAdd % c
	for i := 0; i < d; i++ {
		s += pad
	}
	if r > 0 {
		s += string([]rune(pad)[:r])
	}
	return s
}

func padLeft(s, pad string, n int) string {
	toAdd := n - utf8.RuneCountInString(s)
	if toAdd <= 0 {
		return s
	}
	c := utf8.RuneCountInString(pad)
	if c == 0 {
		return s
	}
	d := toAdd / c
	r := toAdd % c
	for i := 0; i < d; i++ {
		s = pad + s
	}
	if r > 0 {
		s = string([]rune(pad)[c-r:]) + s
	}
	return s
}

func split(s string, r *regexp.Regexp, n int) coretypes.Object {
	indexes := r.FindAllStringIndex(s, n-1)
	lastStart := 0
	result := EmptyVector()
	for _, el := range indexes {
		result = result.Conjoin(coretypes.String{S: s[lastStart:el[0]]})
		lastStart = el[1]
	}
	result = result.Conjoin(coretypes.String{S: s[lastStart:]})
	return result
}

func splitOnStringOrRegex(s string, sep coretypes.Object, n int) coretypes.Object {
	switch sep := sep.(type) {
	case coretypes.String:
		if n == 0 {
			n = -1
		}
		var v []string
		if sep.S == "" {
			// Treat an empty separator as a whitespace split. This mirrors the
			// common text-tokenization fast path and avoids regex/re-seq object churn.
			if n < 0 {
				v = strings.Fields(s)
			} else {
				v = strings.Fields(s)
				if n > 0 && len(v) > n {
					v = v[:n]
				}
			}
		} else {
			v = strings.SplitN(s, sep.S, n)
		}
		result := EmptyArrayVector()
		for _, el := range v {
			result.Append(coretypes.String{S: el})
		}
		return result
	case *coretypes.Regex:
		return split(s, sep.R, n)
	default:
		panic(RT.NewArgTypeError(1, sep, "coretypes.String or Regex"))
	}
}

func join(sep string, seqable coretypes.Seqable) string {
	seq := seqable.Seq()
	var b bytes.Buffer
	for !seq.IsEmpty() {
		b.WriteString(seq.First().ToString(false))
		seq = seq.Rest()
		if !seq.IsEmpty() {
			b.WriteString(sep)
		}
	}
	return b.String()
}

func isBlank(s coretypes.Object) bool {
	if s.Equals(NIL) {
		return true
	}
	str := coretypes.EnsureObjectIsString(s, "").S
	for _, r := range str {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func capitalize(s string) string {
	if len(s) < 2 {
		return strings.ToUpper(s)
	}
	return strings.ToUpper(string([]rune(s)[:1])) + strings.ToLower(string([]rune(s)[1:]))
}

func escape(s string, cmap coretypes.Callable) string {
	var b bytes.Buffer
	for _, r := range s {
		if obj := cmap.Call([]coretypes.Object{coretypes.Char{Ch: r}}); !obj.Equals(NIL) {
			b.WriteString(obj.ToString(false))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func indexOf(s string, value coretypes.Object, from int) coretypes.Object {
	var res int
	if from < 0 {
		from = 0
	}
	runes := []rune(s)
	if from >= len(runes) {
		return NIL
	}
	if from != 0 {
		s = string(runes[from:])
	}
	switch value := value.(type) {
	case coretypes.Char:
		res = strings.IndexRune(s, value.Ch)
	case coretypes.String:
		res = strings.Index(s, value.S)
	default:
		panic(RT.NewArgTypeError(1, value, "coretypes.String or Char"))
	}
	if res == -1 {
		return NIL
	}
	return coretypes.MakeInt(utf8.RuneCountInString(s[:res]) + from)
}

func lastIndexOf(s string, value coretypes.Object, from int) coretypes.Object {
	var res int
	runes := []rune(s)
	if from < 0 {
		return NIL
	}
	if from > len(runes) {
		from = len(runes)
	}
	if from != 0 {
		s = string(runes[:from])
	}
	switch value := value.(type) {
	case coretypes.Char:
		res = strings.LastIndex(s, string(value.Ch))
	case coretypes.String:
		res = strings.LastIndex(s, value.S)
	default:
		panic(RT.NewArgTypeError(1, value, "coretypes.String or Char"))
	}
	if res == -1 {
		return NIL
	}
	return coretypes.MakeInt(utf8.RuneCountInString(s[:res]))
}

func replace(s string, match coretypes.Object, repl string) string {
	switch match := match.(type) {
	case coretypes.String:
		return strings.Replace(s, match.S, repl, -1)
	case *coretypes.Regex:
		return match.R.ReplaceAllString(s, repl)
	default:
		panic(RT.NewArgTypeError(1, match, "coretypes.String or Regex"))
	}
}

func replaceFirst(s string, match coretypes.Object, repl string) string {
	switch match := match.(type) {
	case coretypes.String:
		return strings.Replace(s, match.S, repl, 1)
	case *coretypes.Regex:
		m := match.R.FindStringIndex(s)
		if m == nil {
			return s
		}
		return s[:m[0]] + repl + s[m[1]:]
	default:
		panic(RT.NewArgTypeError(1, match, "coretypes.String or Regex"))
	}
}

func reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func init() {
	newLine, _ = regexp.Compile("\r?\n")
}
