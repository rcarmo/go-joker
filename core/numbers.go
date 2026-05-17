package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math/big"
	"strconv"
)

type (
	IntOps      struct{}
	DoubleOps   struct{}
	BigIntOps   struct{}
	BigFloatOps struct{}
	RatioOps    struct{}
)

const (
	INTEGER_CATEGORY  = iota
	FLOATING_CATEGORY = iota
	RATIO_CATEGORY    = iota
)

const MAX_RUNE = int(^uint32(0) >> 1)
const MIN_RUNE = -MAX_RUNE - 1

var (
	INT_OPS      = IntOps{}
	DOUBLE_OPS   = DoubleOps{}
	BIGINT_OPS   = BigIntOps{}
	BIGFLOAT_OPS = BigFloatOps{}
	RATIO_OPS    = RatioOps{}
)

const maxInt = int(^uint(0) >> 1)
const minInt = -maxInt - 1

var maxIntBig = big.NewInt(int64(maxInt))
var minIntBig = big.NewInt(int64(minInt))

func intOrBigInt(b *big.Int) coretypes.Number {
	if strconv.IntSize == 64 && b.IsInt64() {
		return coretypes.MakeInt(int(b.Int64()))
	}
	if strconv.IntSize == 32 && b.Cmp(minIntBig) >= 0 && b.Cmp(maxIntBig) <= 0 {
		return coretypes.MakeInt(int(b.Int64()))
	}
	return &BigInt{b: new(big.Int).Set(b)}
}

func intOrBigIntWithOriginal(orig string, b *big.Int) coretypes.Number {
	res := intOrBigInt(b)
	if bi, ok := res.(*BigInt); ok {
		bi.Original = orig
		return bi
	}
	return res
}

func bigFloatWithPrec(x, y coretypes.Number, extra uint) *big.Float {
	prec := x.BigFloat().Prec()
	if yp := y.BigFloat().Prec(); yp > prec {
		prec = yp
	}
	if prec < 128 {
		prec = 128
	}
	return new(big.Float).SetPrec(prec + extra).SetMode(big.ToNearestEven)
}

func ratioOrInt(r *big.Rat) coretypes.Number {
	if r.IsInt() {
		return intOrBigInt(r.Num())
	}
	return &Ratio{r: r}
}

func ratioOrIntWithOriginal(orig string, r *big.Rat) coretypes.Number {
	if r.IsInt() {
		return intOrBigIntWithOriginal(orig, r.Num())
	}
	return &Ratio{r: r, Original: orig}
}

func (ops IntOps) Combine(other coretypes.Ops) coretypes.Ops {
	return other
}

func (ops DoubleOps) Combine(other coretypes.Ops) coretypes.Ops {
	switch other.(type) {
	case BigFloatOps:
		return other
	default:
		return ops
	}
}

func (ops BigIntOps) Combine(other coretypes.Ops) coretypes.Ops {
	switch other.(type) {
	case IntOps:
		return ops
	default:
		return other
	}
}

func (ops BigFloatOps) Combine(other coretypes.Ops) coretypes.Ops {
	return ops
}

func (ops RatioOps) Combine(other coretypes.Ops) coretypes.Ops {
	switch other.(type) {
	case DoubleOps, BigFloatOps:
		return other
	default:
		return ops
	}
}

func GetOps(obj Object) coretypes.Ops {
	switch obj.(type) {
	case coretypes.Double:
		return DOUBLE_OPS
	case *BigInt:
		return BIGINT_OPS
	case *BigFloat:
		return BIGFLOAT_OPS
	case *Ratio:
		return RATIO_OPS
	default:
		return INT_OPS
	}
}

// BigInt conversions

func (b *BigInt) Int() coretypes.Int {
	bi := b.BigInt()
	if bi.Cmp(minIntBig) < 0 || bi.Cmp(maxIntBig) > 0 {
		panic(RT.NewError("BigInt value out of native int range: " + b.ToString(false)))
	}
	return coretypes.Int{I: int(bi.Int64())}
}

func (b *BigInt) BigInt() *big.Int {
	return b.b
}

func (b *BigInt) Double() coretypes.Double {
	f, _ := new(big.Float).SetInt(b.BigInt()).Float64()
	return coretypes.Double{D: f}
}

func (b *BigInt) BigFloat() *big.Float {
	res := big.Float{}
	return res.SetInt(b.BigInt())
}

func (b *BigInt) Ratio() *big.Rat {
	res := big.Rat{}
	return res.SetInt(b.BigInt())
}

// BigFloat conversions

func (b *BigFloat) Int() coretypes.Int {
	f, _ := b.BigFloat().Float64()
	return coretypes.Int{I: int(f)}
}

func (b *BigFloat) BigInt() *big.Int {
	bi, _ := b.BigFloat().Int(nil)
	return bi
}

func (b *BigFloat) Double() coretypes.Double {
	f, _ := b.BigFloat().Float64()
	return coretypes.Double{D: f}
}

func (b *BigFloat) BigFloat() *big.Float {
	return b.b
}

func (b *BigFloat) Ratio() *big.Rat {
	res := big.Rat{}
	return res.SetFloat64(float64(b.Double().D))
}

// Ratio conversions

func (r *Ratio) Int() coretypes.Int {
	f, _ := r.Ratio().Float64()
	return coretypes.Int{I: int(f)}
}

func (r *Ratio) BigInt() *big.Int {
	f, _ := r.Ratio().Float64()
	return big.NewInt(int64(f))
}

func (r *Ratio) Double() coretypes.Double {
	f, _ := r.Ratio().Float64()
	return coretypes.Double{D: f}
}

func (r *Ratio) BigFloat() *big.Float {
	f, _ := r.Ratio().Float64()
	return big.NewFloat(f)
}

func (r *Ratio) Ratio() *big.Rat {
	return r.r
}

// coretypes.Ops

// Add

func (ops IntOps) Add(x, y coretypes.Number) coretypes.Number {
	xi, yi := x.Int().I, y.Int().I
	if (yi > 0 && xi > maxInt-yi) || (yi < 0 && xi < minInt-yi) {
		b := new(big.Int).Add(big.NewInt(int64(xi)), big.NewInt(int64(yi)))
		return &BigInt{b: b}
	}
	return coretypes.Int{I: xi + yi}
}

func (ops DoubleOps) Add(x, y coretypes.Number) coretypes.Number {
	return coretypes.Double{D: x.Double().D + y.Double().D}
}

func (ops BigIntOps) Add(x, y coretypes.Number) coretypes.Number {
	b := &big.Int{}
	b.Add(x.BigInt(), y.BigInt())
	res := BigInt{b: b}
	return &res
}

func (ops BigFloatOps) Add(x, y coretypes.Number) coretypes.Number {
	b := bigFloatWithPrec(x, y, 1)
	b.Add(x.BigFloat(), y.BigFloat())
	return &BigFloat{b: b}
}

func (ops RatioOps) Add(x, y coretypes.Number) coretypes.Number {
	r := big.Rat{}
	r.Add(x.Ratio(), y.Ratio())
	return ratioOrInt(&r)
}

// Subtract

func (ops IntOps) Subtract(x, y coretypes.Number) coretypes.Number {
	xi, yi := x.Int().I, y.Int().I
	if (yi < 0 && xi > maxInt+yi) || (yi > 0 && xi < minInt+yi) {
		b := new(big.Int).Sub(big.NewInt(int64(xi)), big.NewInt(int64(yi)))
		return &BigInt{b: b}
	}
	return coretypes.Int{I: xi - yi}
}

func (ops DoubleOps) Subtract(x, y coretypes.Number) coretypes.Number {
	return coretypes.Double{D: x.Double().D - y.Double().D}
}

func (ops BigIntOps) Subtract(x, y coretypes.Number) coretypes.Number {
	b := &big.Int{}
	b.Sub(x.BigInt(), y.BigInt())
	res := BigInt{b: b}
	return &res
}

func (ops BigFloatOps) Subtract(x, y coretypes.Number) coretypes.Number {
	b := bigFloatWithPrec(x, y, 1)
	b.Sub(x.BigFloat(), y.BigFloat())
	return &BigFloat{b: b}
}

func (ops RatioOps) Subtract(x, y coretypes.Number) coretypes.Number {
	r := &big.Rat{}
	r.Sub(x.Ratio(), y.Ratio())
	return ratioOrInt(r)
}

// Multiply

func (ops IntOps) Multiply(x, y coretypes.Number) coretypes.Number {
	xi, yi := x.Int().I, y.Int().I
	b := new(big.Int).Mul(big.NewInt(int64(xi)), big.NewInt(int64(yi)))
	return intOrBigInt(b)
}

func (ops DoubleOps) Multiply(x, y coretypes.Number) coretypes.Number {
	return coretypes.Double{D: x.Double().D * y.Double().D}
}

func (ops BigIntOps) Multiply(x, y coretypes.Number) coretypes.Number {
	b := &big.Int{}
	b.Mul(x.BigInt(), y.BigInt())
	res := BigInt{b: b}
	return &res
}

func (ops BigFloatOps) Multiply(x, y coretypes.Number) coretypes.Number {
	b := bigFloatWithPrec(x, y, x.BigFloat().Prec()+y.BigFloat().Prec())
	b.Mul(x.BigFloat(), y.BigFloat())
	return &BigFloat{b: b}
}

func (ops RatioOps) Multiply(x, y coretypes.Number) coretypes.Number {
	r := big.Rat{}
	r.Mul(x.Ratio(), y.Ratio())
	return ratioOrInt(&r)
}

func panicOnZero(ops coretypes.Ops, n coretypes.Number) {
	if ops.IsZero(n) {
		panic(RT.NewError("Division by zero"))
	}
}

// Divide

func (ops IntOps) Divide(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	b := big.NewRat(int64(x.Int().I), int64(y.Int().I))
	return ratioOrInt(b)
}

func (ops DoubleOps) Divide(x, y coretypes.Number) coretypes.Number {
	return coretypes.Double{D: x.Double().D / y.Double().D}
}

func (ops BigIntOps) Divide(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	b := &big.Rat{}
	b.Quo(x.Ratio(), y.Ratio())
	if b.IsInt() {
		res := BigInt{b: b.Num()}
		return &res
	}
	res := Ratio{r: b}
	return &res
}

func (ops BigFloatOps) Divide(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	b := bigFloatWithPrec(x, y, 64)
	b.Quo(x.BigFloat(), y.BigFloat())
	return &BigFloat{b: b}
}

func (ops RatioOps) Divide(x, y coretypes.Number) coretypes.Number {
	if y.Ratio().Num().Int64() == 0 {
		panic(RT.NewError("Division by zero"))
	}
	r := big.Rat{}
	r.Quo(x.Ratio(), y.Ratio())
	return ratioOrInt(&r)
}

// Quotient

func (ops IntOps) Quotient(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	return coretypes.Int{I: x.Int().I / y.Int().I}
}

func (ops DoubleOps) Quotient(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	z := x.Double().D / y.Double().D
	return coretypes.Double{D: float64(int64(z))}
}

func (ops BigIntOps) Quotient(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	z := &big.Int{}
	z.Quo(x.BigInt(), y.BigInt())
	return &BigInt{b: z}
}

func (ops BigFloatOps) Quotient(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	z := &big.Float{}
	i, _ := z.Quo(x.BigFloat(), y.BigFloat()).Int64()
	return &BigFloat{b: z.SetInt64(i)}
}

func (ops RatioOps) Quotient(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	z := &big.Rat{}
	f, _ := z.Quo(x.Ratio(), y.Ratio()).Float64()
	return &BigInt{b: big.NewInt(int64(f))}
}

// Remainder

func (ops IntOps) Rem(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	return coretypes.Int{I: x.Int().I % y.Int().I}
}

func (ops DoubleOps) Rem(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	n := x.Double().D
	d := y.Double().D
	z := n / d
	return coretypes.Double{D: n - float64(int64(z))*d}
}

func (ops BigIntOps) Rem(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	z := &big.Int{}
	z.Rem(x.BigInt(), y.BigInt())
	return &BigInt{b: z}
}

func (ops BigFloatOps) Rem(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	n := x.BigFloat()
	d := y.BigFloat()
	z := &big.Float{}
	i, _ := z.Quo(n, d).Int64()
	d.Mul(d, big.NewFloat(float64(i)))
	z.Sub(n, d)
	return &BigFloat{b: z}
}

func (ops RatioOps) Rem(x, y coretypes.Number) coretypes.Number {
	panicOnZero(ops, y)
	n := x.Ratio()
	d := y.Ratio()
	z := big.Rat{}
	f, _ := z.Quo(n, d).Float64()
	d.Mul(d, big.NewRat(int64(f), 1))
	z.Sub(n, d)
	return ratioOrInt(&z)
}

// IsZero

func (ops IntOps) IsZero(x coretypes.Number) bool {
	return x.Int().I == 0
}

func (ops DoubleOps) IsZero(x coretypes.Number) bool {
	return x.Double().D == 0
}

func (ops BigIntOps) IsZero(x coretypes.Number) bool {
	return x.BigInt().Sign() == 0
}

func (ops BigFloatOps) IsZero(x coretypes.Number) bool {
	return x.BigFloat().Sign() == 0
}

func (ops RatioOps) IsZero(x coretypes.Number) bool {
	return x.Ratio().Sign() == 0
}

// Lt

func (ops IntOps) Lt(x coretypes.Number, y coretypes.Number) bool {
	return x.Int().I < y.Int().I
}

func (ops DoubleOps) Lt(x coretypes.Number, y coretypes.Number) bool {
	return x.Double().D < y.Double().D
}

func (ops BigIntOps) Lt(x coretypes.Number, y coretypes.Number) bool {
	return x.BigInt().Cmp(y.BigInt()) < 0
}

func (ops BigFloatOps) Lt(x coretypes.Number, y coretypes.Number) bool {
	return x.BigFloat().Cmp(y.BigFloat()) < 0
}

func (ops RatioOps) Lt(x coretypes.Number, y coretypes.Number) bool {
	return x.Ratio().Cmp(y.Ratio()) < 0
}

// Lte

func (ops IntOps) Lte(x coretypes.Number, y coretypes.Number) bool {
	return x.Int().I <= y.Int().I
}

func (ops DoubleOps) Lte(x coretypes.Number, y coretypes.Number) bool {
	return x.Double().D <= y.Double().D
}

func (ops BigIntOps) Lte(x coretypes.Number, y coretypes.Number) bool {
	return x.BigInt().Cmp(y.BigInt()) <= 0
}

func (ops BigFloatOps) Lte(x coretypes.Number, y coretypes.Number) bool {
	return x.BigFloat().Cmp(y.BigFloat()) <= 0
}

func (ops RatioOps) Lte(x coretypes.Number, y coretypes.Number) bool {
	return x.Ratio().Cmp(y.Ratio()) <= 0
}

// Gt

func (ops IntOps) Gt(x coretypes.Number, y coretypes.Number) bool {
	return x.Int().I > y.Int().I
}

func (ops DoubleOps) Gt(x coretypes.Number, y coretypes.Number) bool {
	return x.Double().D > y.Double().D
}

func (ops BigIntOps) Gt(x coretypes.Number, y coretypes.Number) bool {
	return x.BigInt().Cmp(y.BigInt()) > 0
}

func (ops BigFloatOps) Gt(x coretypes.Number, y coretypes.Number) bool {
	return x.BigFloat().Cmp(y.BigFloat()) > 0
}

func (ops RatioOps) Gt(x coretypes.Number, y coretypes.Number) bool {
	return x.Ratio().Cmp(y.Ratio()) > 0
}

// Gte

func (ops IntOps) Gte(x coretypes.Number, y coretypes.Number) bool {
	return x.Int().I >= y.Int().I
}

func (ops DoubleOps) Gte(x coretypes.Number, y coretypes.Number) bool {
	return x.Double().D >= y.Double().D
}

func (ops BigIntOps) Gte(x coretypes.Number, y coretypes.Number) bool {
	return x.BigInt().Cmp(y.BigInt()) >= 0
}

func (ops BigFloatOps) Gte(x coretypes.Number, y coretypes.Number) bool {
	return x.BigFloat().Cmp(y.BigFloat()) >= 0
}

func (ops RatioOps) Gte(x coretypes.Number, y coretypes.Number) bool {
	return x.Ratio().Cmp(y.Ratio()) >= 0
}

// Eq

func (ops IntOps) Eq(x coretypes.Number, y coretypes.Number) bool {
	return x.Int().I == y.Int().I
}

func (ops DoubleOps) Eq(x coretypes.Number, y coretypes.Number) bool {
	return x.Double().D == y.Double().D
}

func (ops BigIntOps) Eq(x coretypes.Number, y coretypes.Number) bool {
	return x.BigInt().Cmp(y.BigInt()) == 0
}

func (ops BigFloatOps) Eq(x coretypes.Number, y coretypes.Number) bool {
	return x.BigFloat().Cmp(y.BigFloat()) == 0
}

func (ops RatioOps) Eq(x coretypes.Number, y coretypes.Number) bool {
	return x.Ratio().Cmp(y.Ratio()) == 0
}

func numbersEq(x coretypes.Number, y coretypes.Number) bool {
	return GetOps(x).Combine(GetOps(y)).Eq(x, y)
}

func CompareNumbers(x coretypes.Number, y coretypes.Number) int {
	ops := GetOps(x).Combine(GetOps(y))
	if ops.Lt(x, y) {
		return -1
	}
	if ops.Lt(y, x) {
		return 1
	}
	return 0
}

func Max(x coretypes.Number, y coretypes.Number) coretypes.Number {
	ops := GetOps(x).Combine(GetOps(y))
	if ops.Lt(x, y) {
		return y
	}
	return x
}

func Min(x coretypes.Number, y coretypes.Number) coretypes.Number {
	ops := GetOps(x).Combine(GetOps(y))
	if ops.Lt(x, y) {
		return x
	}
	return y
}

// Precision

func (n *BigInt) Precision() *big.Int {
	return MakeMathBigIntFromInt(n.b.BitLen())
}

func (n *BigFloat) Precision() *big.Int {
	return MakeMathBigIntFromUint(n.b.Prec())
}

func category(x coretypes.Number) int {
	switch x.(type) {
	case *BigFloat:
		return FLOATING_CATEGORY
	case coretypes.Double:
		return FLOATING_CATEGORY
	case *Ratio:
		return RATIO_CATEGORY
	default:
		return INTEGER_CATEGORY
	}
}
