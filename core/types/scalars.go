package types

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/big"
	"regexp"
	"time"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	"github.com/rcarmo/go-joker/core/types/numerical"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

var RuntimeTypes *Types

type Char struct {
	InfoHolder
	Ch rune
}
type Double struct{ D float64 }
type Int struct{ I int }
type Boolean struct {
	InfoHolder
	B bool
}
type Time struct {
	InfoHolder
	T time.Time
}

type Regex struct {
	InfoHolder
	R *regexp.Regexp
}

type Comment struct {
	InfoHolder
	C string
}

func MakeBoolean(b bool) Boolean        { return Boolean{B: b} }
func MakeTime(t time.Time) Time         { return Time{T: t} }
func MakeDouble(d float64) Double       { return Double{D: d} }
func MakeInt(i int) Int                 { return Int{I: i} }
func MakeChar(r rune) Char              { return Char{Ch: r} }
func MakeRegex(r *regexp.Regexp) *Regex { return &Regex{R: r} }

func (c Char) ToString(escape bool) string {
	if escape {
		return corestr.EscapeRune(c.Ch)
	}
	return corestr.String(c.Ch)
}
func (c Char) Equals(other interface{}) bool { o, ok := other.(Char); return ok && c.Ch == o.Ch }
func (c Char) GetType() *Type                { return RuntimeTypes.Char }
func (c Char) Native() interface{}           { return c.Ch }
func (c Char) Hash() uint32                  { h := hashutil.New32(); h.Write([]byte(string(c.Ch))); return h.Sum32() }
func (c Char) Compare(other Object) int {
	c2 := other.(Char)
	if c.Ch < c2.Ch {
		return -1
	}
	if c2.Ch < c.Ch {
		return 1
	}
	return 0
}

func (d Double) GetInfo() *ObjectInfo { return nil }
func (d Double) ToString(escape bool) string {
	dbl := d.D
	if math.IsInf(dbl, 1) {
		return "##Inf"
	}
	if math.IsInf(dbl, -1) {
		return "##-Inf"
	}
	if math.IsNaN(dbl) {
		return "##NaN"
	}
	res := fmt.Sprintf("%g", dbl)
	if numerical.NeedsDecimalSuffix(res) {
		return res + ".0"
	}
	return res
}
func (d Double) Equals(other interface{}) bool { o, ok := numericFloat(other); return ok && d.D == o }
func (d Double) GetType() *Type                { return RuntimeTypes.Double }
func (d Double) Native() interface{}           { return d.D }
func (d Double) Hash() uint32 {
	h := hashutil.New32()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, math.Float64bits(d.D))
	h.Write(b)
	return h.Sum32()
}
func (d Double) Compare(other Object) int { o, _ := numericFloat(other); return cmpFloat(d.D, o) }
func (d Double) Int() Int                 { return Int{I: int(d.D)} }
func (d Double) BigInt() *big.Int         { return big.NewInt(int64(d.D)) }
func (d Double) Double() Double           { return d }
func (d Double) BigFloat() *big.Float     { return big.NewFloat(float64(d.D)) }
func (d Double) Ratio() *big.Rat          { res := big.Rat{}; return res.SetFloat64(float64(d.D)) }

func (i Int) GetInfo() *ObjectInfo        { return nil }
func (i Int) ToString(escape bool) string { return corestr.Int(i.I) }
func (i Int) Equals(other interface{}) bool {
	o, ok := numericFloat(other)
	return ok && float64(i.I) == o
}
func (i Int) GetType() *Type      { return RuntimeTypes.Int }
func (i Int) Native() interface{} { return i.I }
func (i Int) Hash() uint32 {
	h := hashutil.New32()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(i.I))
	h.Write(b)
	return h.Sum32()
}
func (i Int) Compare(other Object) int { o, _ := numericFloat(other); return cmpFloat(float64(i.I), o) }
func (i Int) Int() Int                 { return i }
func (i Int) Double() Double           { return Double{D: float64(i.I)} }
func (i Int) BigInt() *big.Int         { return big.NewInt(int64(i.I)) }
func (i Int) BigFloat() *big.Float     { return big.NewFloat(float64(i.I)) }
func (i Int) Ratio() *big.Rat          { return big.NewRat(int64(i.I), 1) }

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

func numericFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case Int:
		return float64(n.I), true
	case Double:
		return n.D, true
	default:
		return 0, false
	}
}
func cmpFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

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

func (c Comment) ToString(escape bool) string   { return c.C }
func (c Comment) Equals(other interface{}) bool { return false }
func (c Comment) GetType() *Type                { return RuntimeTypes.String }
func (c Comment) Hash() uint32 {
	h := hashutil.New32()
	h.Write([]byte(c.C))
	return h.Sum32()
}
func (c Comment) WithInfo(info *ObjectInfo) Object { c.Info = info; return c }
