package types

import (
	"encoding/gob"
	"math/big"
	"strconv"

	"github.com/rcarmo/go-joker/core/hashutil"
	"github.com/rcarmo/go-joker/core/numutil"
)

// Number is the numeric object protocol shared by scalar and big numeric values.
type Number interface {
	Object
	GetType() *Type
	Int() Int
	Double() Double
	BigInt() *big.Int
	BigFloat() *big.Float
	Ratio() *big.Rat
}

// Precision reports the precision of numeric values that expose it.
type Precision interface {
	Precision() *big.Int
}

// Ops describes arithmetic operations for a numeric promotion category.
type Ops interface {
	Combine(ops Ops) Ops
	Add(Number, Number) Number
	Subtract(Number, Number) Number
	Multiply(Number, Number) Number
	Divide(Number, Number) Number
	IsZero(Number) bool
	Lt(Number, Number) bool
	Lte(Number, Number) bool
	Gt(Number, Number) bool
	Gte(Number, Number) bool
	Eq(Number, Number) bool
	Quotient(Number, Number) Number
	Rem(Number, Number) Number
}

// MakeMathBigIntFromInt returns a math/big.Int for a native int.
func MakeMathBigIntFromInt(i int) *big.Int { return MakeMathBigIntFromInt64(int64(i)) }

// MakeMathBigIntFromInt64 returns a math/big.Int for an int64.
func MakeMathBigIntFromInt64(i int64) *big.Int { return big.NewInt(i) }

// MakeMathBigIntFromUint returns a math/big.Int for a native uint.
func MakeMathBigIntFromUint(b uint) *big.Int { return MakeMathBigIntFromUint64(uint64(b)) }

// MakeMathBigIntFromUint64 returns a math/big.Int for a uint64.
func MakeMathBigIntFromUint64(b uint64) *big.Int {
	bigint := big.NewInt(0)
	bigint.SetUint64(b)
	return bigint
}

type BigInt struct {
	InfoHolder
	B        *big.Int
	Original string
}

type BigFloat struct {
	InfoHolder
	B        *big.Float
	Original string
}

type Ratio struct {
	InfoHolder
	R        *big.Rat
	Original string
}

var FormatMode bool
var NumberCompare func(Number, Number) int
var NumberEquals func(Number, interface{}) bool
var BigIntNativeBoundsError func(*BigInt) string

func MakeBigInt(b *big.Int) *BigInt       { return &BigInt{B: b} }
func MakeRatio(r *big.Rat) *Ratio         { return &Ratio{R: r} }
func MakeBigFloat(b *big.Float) *BigFloat { return &BigFloat{B: b} }

func MakeBigFloatWithOrig(s, orig string) (*BigFloat, bool) {
	prec := numutil.ComputeFloatPrecision(s)
	f := new(big.Float)
	f.SetPrec(prec)
	if _, ok := f.SetString(s); ok {
		return &BigFloat{B: f, Original: orig}, true
	}
	return nil, false
}

func (rat *Ratio) ToString(escape bool) string { return rat.R.String() }
func (rat *Ratio) Equals(other interface{}) bool {
	if NumberEquals != nil {
		return NumberEquals(rat, other)
	}
	n, ok := other.(Number)
	return ok && rat.Ratio().Cmp(n.Ratio()) == 0
}
func (rat *Ratio) GetType() *Type           { return RuntimeTypes.Ratio }
func (rat *Ratio) Hash() uint32             { return hashBig(rat.R) }
func (rat *Ratio) Compare(other Object) int { return compareNumbers(rat, other.(Number)) }

func (bi *BigInt) ToString(escape bool) string {
	if FormatMode && bi.Original != "" {
		return bi.Original
	}
	return bi.B.String() + "N"
}
func (bi *BigInt) Equals(other interface{}) bool {
	if NumberEquals != nil {
		return NumberEquals(bi, other)
	}
	n, ok := other.(Number)
	return ok && bi.Ratio().Cmp(n.Ratio()) == 0
}
func (bi *BigInt) GetType() *Type           { return RuntimeTypes.BigInt }
func (bi *BigInt) Hash() uint32             { return hashBig(bi.B) }
func (bi *BigInt) Compare(other Object) int { return compareNumbers(bi, other.(Number)) }

func (bf *BigFloat) ToString(escape bool) string {
	if FormatMode && bf.Original != "" {
		return bf.Original
	}
	if bf.B.IsInf() {
		if bf.B.Signbit() {
			return "##-Inf"
		}
		return "##Inf"
	}
	return bf.B.Text('g', -1) + "M"
}
func (bf *BigFloat) Equals(other interface{}) bool {
	if NumberEquals != nil {
		return NumberEquals(bf, other)
	}
	n, ok := other.(Number)
	return ok && bf.BigFloat().Cmp(n.BigFloat()) == 0
}
func (bf *BigFloat) GetType() *Type           { return RuntimeTypes.BigFloat }
func (bf *BigFloat) Hash() uint32             { return hashBig(bf.B) }
func (bf *BigFloat) Compare(other Object) int { return compareNumbers(bf, other.(Number)) }

func (b *BigInt) Int() Int {
	bi := b.BigInt()
	if bi.Cmp(minIntBig) < 0 || bi.Cmp(maxIntBig) > 0 {
		if BigIntNativeBoundsError != nil {
			panic(BigIntNativeBoundsError(b))
		}
		panic("BigInt value out of native int range: " + b.ToString(false))
	}
	return Int{I: int(bi.Int64())}
}
func (b *BigInt) BigInt() *big.Int { return b.B }
func (b *BigInt) Double() Double {
	f, _ := new(big.Float).SetInt(b.BigInt()).Float64()
	return Double{D: f}
}
func (b *BigInt) BigFloat() *big.Float   { res := big.Float{}; return res.SetInt(b.BigInt()) }
func (b *BigInt) Ratio() *big.Rat        { res := big.Rat{}; return res.SetInt(b.BigInt()) }
func (b *BigFloat) Int() Int             { f, _ := b.BigFloat().Float64(); return Int{I: int(f)} }
func (b *BigFloat) BigInt() *big.Int     { bi, _ := b.BigFloat().Int(nil); return bi }
func (b *BigFloat) Double() Double       { f, _ := b.BigFloat().Float64(); return Double{D: f} }
func (b *BigFloat) BigFloat() *big.Float { return b.B }
func (b *BigFloat) Ratio() *big.Rat      { res := big.Rat{}; return res.SetFloat64(float64(b.Double().D)) }
func (r *Ratio) Int() Int                { f, _ := r.Ratio().Float64(); return Int{I: int(f)} }
func (r *Ratio) BigInt() *big.Int        { f, _ := r.Ratio().Float64(); return big.NewInt(int64(f)) }
func (r *Ratio) Double() Double          { f, _ := r.Ratio().Float64(); return Double{D: f} }
func (r *Ratio) BigFloat() *big.Float    { f, _ := r.Ratio().Float64(); return big.NewFloat(f) }
func (r *Ratio) Ratio() *big.Rat         { return r.R }

func compareNumbers(x, y Number) int {
	if NumberCompare != nil {
		return NumberCompare(x, y)
	}
	return x.BigFloat().Cmp(y.BigFloat())
}
func hashBig(v gob.GobEncoder) uint32 { return hashutil.GobEncoder(v) }

var maxInt = int(^uint(0) >> 1)
var minInt = -maxInt - 1
var maxIntBig = big.NewInt(int64(maxInt))
var minIntBig = big.NewInt(int64(minInt))
var _ = strconv.IntSize

func (n *BigInt) Precision() *big.Int   { return MakeMathBigIntFromInt(n.B.BitLen()) }
func (n *BigFloat) Precision() *big.Int { return MakeMathBigIntFromUint(n.B.Prec()) }
