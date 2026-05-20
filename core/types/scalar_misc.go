package types

import (
	"fmt"
	"io"
	"regexp"
	"time"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
)

type Boolean struct {
	InfoHolder
	B bool
}

func MakeBoolean(b bool) Boolean { return Boolean{B: b} }

func (b Boolean) ToString(escape bool) string   { return fmt.Sprintf("%t", b.B) }
func (b Boolean) Equals(other interface{}) bool { o, ok := other.(Boolean); return ok && b.B == o.B }
func (b Boolean) GetType() *Type                { return RuntimeTypes.Boolean }
func (b Boolean) Native() interface{}           { return b.B }
func (b Boolean) Hash() uint32 {
	h := hashutil.New32()
	bs := []byte{0}
	if b.B {
		bs[0] = 1
	}
	h.Write(bs)
	return h.Sum32()
}
func (b Boolean) Compare(other Object) int {
	b2 := other.(Boolean)
	if b.B == b2.B {
		return 0
	}
	if b.B {
		return 1
	}
	return -1
}

type Comment struct {
	InfoHolder
	C string
}

func (c Comment) ToString(escape bool) string   { return c.C }
func (c Comment) Equals(other interface{}) bool { return false }
func (c Comment) GetType() *Type                { return RuntimeTypes.String }
func (c Comment) Hash() uint32 {
	h := hashutil.New32()
	h.Write([]byte(c.C))
	return h.Sum32()
}
func (c Comment) WithInfo(info *ObjectInfo) Object { c.Info = info; return c }

type Regex struct {
	InfoHolder
	R *regexp.Regexp
}

func MakeRegex(r *regexp.Regexp) *Regex { return &Regex{R: r} }

func (rx *Regex) ToString(escape bool) string {
	if escape {
		return "#\"" + rx.R.String() + "\""
	}
	return rx.R.String()
}

func (rx *Regex) Print(w io.Writer, printReadably bool) { fmt.Fprint(w, rx.ToString(true)) }

func (rx *Regex) Equals(other interface{}) bool {
	switch other := other.(type) {
	case *Regex:
		return rx.R == other.R
	default:
		return false
	}
}

func (rx *Regex) GetType() *Type                   { return RuntimeTypes.Regex }
func (rx *Regex) Hash() uint32                     { return hashutil.Ptr(uintptr(unsafe.Pointer(rx.R))) }
func (rx *Regex) WithInfo(info *ObjectInfo) Object { rx.Info = info; return rx }

type Time struct {
	InfoHolder
	T time.Time
}

func MakeTime(t time.Time) Time { return Time{T: t} }

func (t Time) ToString(escape bool) string   { return t.T.String() }
func (t Time) Equals(other interface{}) bool { o, ok := other.(Time); return ok && t.T.Equal(o.T) }
func (t Time) GetType() *Type                { return RuntimeTypes.Time }
func (t Time) Native() interface{}           { return t.T }
func (t Time) Hash() uint32                  { return hashutil.GobEncoder(t.T) }
func (t Time) Compare(other Object) int {
	t2 := other.(Time)
	if t.T.Equal(t2.T) {
		return 0
	}
	if t2.T.Before(t.T) {
		return 1
	}
	return -1
}
