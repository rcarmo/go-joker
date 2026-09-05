package types

import (
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

const MaxInt = int(^uint(0) >> 1)
const MinInt = -MaxInt - 1

var MaxIntBig = big.NewInt(int64(MaxInt))
var MinIntBig = big.NewInt(int64(MinInt))

func IntOrBigInt(b *big.Int) Number {
	if strconv.IntSize == 64 && b.IsInt64() {
		return MakeInt(int(b.Int64()))
	}
	if strconv.IntSize == 32 && b.Cmp(MinIntBig) >= 0 && b.Cmp(MaxIntBig) <= 0 {
		return MakeInt(int(b.Int64()))
	}
	return &BigInt{B: new(big.Int).Set(b)}
}

func IntOrBigIntWithOriginal(orig string, b *big.Int) Number {
	res := IntOrBigInt(b)
	if bi, ok := res.(*BigInt); ok {
		bi.Original = orig
		return bi
	}
	return res
}

func BigFloatWithPrec(x, y Number, extra uint) *big.Float {
	prec := x.BigFloat().Prec()
	if yp := y.BigFloat().Prec(); yp > prec {
		prec = yp
	}
	if prec < 128 {
		prec = 128
	}
	return new(big.Float).SetPrec(prec + extra).SetMode(big.ToNearestEven)
}

func RatioOrInt(r *big.Rat) Number {
	if r.IsInt() {
		return IntOrBigInt(r.Num())
	}
	return &Ratio{R: r}
}

func RatioOrIntWithOriginal(orig string, r *big.Rat) Number {
	if r.IsInt() {
		return IntOrBigIntWithOriginal(orig, r.Num())
	}
	return &Ratio{R: r, Original: orig}
}

func (ops IntOps) Combine(other Ops) Ops {
	return other
}

func (ops DoubleOps) Combine(other Ops) Ops {
	switch other.(type) {
	case BigFloatOps:
		return other
	default:
		return ops
	}
}

func (ops BigIntOps) Combine(other Ops) Ops {
	switch other.(type) {
	case IntOps:
		return ops
	default:
		return other
	}
}

func (ops BigFloatOps) Combine(other Ops) Ops {
	return ops
}

func (ops RatioOps) Combine(other Ops) Ops {
	switch other.(type) {
	case DoubleOps, BigFloatOps:
		return other
	default:
		return ops
	}
}

func GetOps(obj Number) Ops {
	switch obj.(type) {
	case Double:
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

// Ops

// Add

func (ops IntOps) Add(x, y Number) Number {
	xi, yi := x.Int().I, y.Int().I
	if (yi > 0 && xi > MaxInt-yi) || (yi < 0 && xi < MinInt-yi) {
		b := new(big.Int).Add(big.NewInt(int64(xi)), big.NewInt(int64(yi)))
		return &BigInt{B: b}
	}
	return Int{I: xi + yi}
}

func (ops DoubleOps) Add(x, y Number) Number {
	return Double{D: x.Double().D + y.Double().D}
}

func (ops BigIntOps) Add(x, y Number) Number {
	b := &big.Int{}
	b.Add(x.BigInt(), y.BigInt())
	res := BigInt{B: b}
	return &res
}

func (ops BigFloatOps) Add(x, y Number) Number {
	b := BigFloatWithPrec(x, y, 1)
	b.Add(x.BigFloat(), y.BigFloat())
	return &BigFloat{B: b}
}

func (ops RatioOps) Add(x, y Number) Number {
	r := big.Rat{}
	r.Add(x.Ratio(), y.Ratio())
	return RatioOrInt(&r)
}

// Subtract

func (ops IntOps) Subtract(x, y Number) Number {
	xi, yi := x.Int().I, y.Int().I
	if (yi < 0 && xi > MaxInt+yi) || (yi > 0 && xi < MinInt+yi) {
		b := new(big.Int).Sub(big.NewInt(int64(xi)), big.NewInt(int64(yi)))
		return &BigInt{B: b}
	}
	return Int{I: xi - yi}
}

func (ops DoubleOps) Subtract(x, y Number) Number {
	return Double{D: x.Double().D - y.Double().D}
}

func (ops BigIntOps) Subtract(x, y Number) Number {
	b := &big.Int{}
	b.Sub(x.BigInt(), y.BigInt())
	res := BigInt{B: b}
	return &res
}

func (ops BigFloatOps) Subtract(x, y Number) Number {
	b := BigFloatWithPrec(x, y, 1)
	b.Sub(x.BigFloat(), y.BigFloat())
	return &BigFloat{B: b}
}

func (ops RatioOps) Subtract(x, y Number) Number {
	r := &big.Rat{}
	r.Sub(x.Ratio(), y.Ratio())
	return RatioOrInt(r)
}

// Multiply

func (ops IntOps) Multiply(x, y Number) Number {
	xi, yi := x.Int().I, y.Int().I
	product := xi * yi
	// Division verifies a non-overflowing product, except MinInt / -1,
	// whose machine result also wraps. Handle that pair explicitly.
	if xi == 0 || (xi != -1 || yi != MinInt) && (yi != -1 || xi != MinInt) && product/xi == yi {
		return Int{I: product}
	}
	b := new(big.Int).Mul(big.NewInt(int64(xi)), big.NewInt(int64(yi)))
	return IntOrBigInt(b)
}

func (ops DoubleOps) Multiply(x, y Number) Number {
	return Double{D: x.Double().D * y.Double().D}
}

func (ops BigIntOps) Multiply(x, y Number) Number {
	b := &big.Int{}
	b.Mul(x.BigInt(), y.BigInt())
	res := BigInt{B: b}
	return &res
}

func (ops BigFloatOps) Multiply(x, y Number) Number {
	b := BigFloatWithPrec(x, y, x.BigFloat().Prec()+y.BigFloat().Prec())
	b.Mul(x.BigFloat(), y.BigFloat())
	return &BigFloat{B: b}
}

func (ops RatioOps) Multiply(x, y Number) Number {
	r := big.Rat{}
	r.Mul(x.Ratio(), y.Ratio())
	return RatioOrInt(&r)
}

func PanicOnZero(ops Ops, n Number) {
	if ops.IsZero(n) {
		panicNumericError("Division by zero")
	}
}

// Divide

func (ops IntOps) Divide(x, y Number) Number {
	PanicOnZero(ops, y)
	b := big.NewRat(int64(x.Int().I), int64(y.Int().I))
	return RatioOrInt(b)
}

func (ops DoubleOps) Divide(x, y Number) Number {
	return Double{D: x.Double().D / y.Double().D}
}

func (ops BigIntOps) Divide(x, y Number) Number {
	PanicOnZero(ops, y)
	b := &big.Rat{}
	b.Quo(x.Ratio(), y.Ratio())
	if b.IsInt() {
		res := BigInt{B: b.Num()}
		return &res
	}
	res := Ratio{R: b}
	return &res
}

func (ops BigFloatOps) Divide(x, y Number) Number {
	PanicOnZero(ops, y)
	b := BigFloatWithPrec(x, y, 64)
	b.Quo(x.BigFloat(), y.BigFloat())
	return &BigFloat{B: b}
}

func (ops RatioOps) Divide(x, y Number) Number {
	if y.Ratio().Num().Int64() == 0 {
		panicNumericError("Division by zero")
	}
	r := big.Rat{}
	r.Quo(x.Ratio(), y.Ratio())
	return RatioOrInt(&r)
}

// Quotient

func (ops IntOps) Quotient(x, y Number) Number {
	PanicOnZero(ops, y)
	return Int{I: x.Int().I / y.Int().I}
}

func (ops DoubleOps) Quotient(x, y Number) Number {
	PanicOnZero(ops, y)
	z := x.Double().D / y.Double().D
	return Double{D: float64(int64(z))}
}

func (ops BigIntOps) Quotient(x, y Number) Number {
	PanicOnZero(ops, y)
	z := &big.Int{}
	z.Quo(x.BigInt(), y.BigInt())
	return &BigInt{B: z}
}

func (ops BigFloatOps) Quotient(x, y Number) Number {
	PanicOnZero(ops, y)
	z := &big.Float{}
	i, _ := z.Quo(x.BigFloat(), y.BigFloat()).Int64()
	return &BigFloat{B: z.SetInt64(i)}
}

func (ops RatioOps) Quotient(x, y Number) Number {
	PanicOnZero(ops, y)
	z := &big.Rat{}
	f, _ := z.Quo(x.Ratio(), y.Ratio()).Float64()
	return &BigInt{B: big.NewInt(int64(f))}
}

// Remainder

func (ops IntOps) Rem(x, y Number) Number {
	PanicOnZero(ops, y)
	return Int{I: x.Int().I % y.Int().I}
}

func (ops DoubleOps) Rem(x, y Number) Number {
	PanicOnZero(ops, y)
	n := x.Double().D
	d := y.Double().D
	z := n / d
	return Double{D: n - float64(int64(z))*d}
}

func (ops BigIntOps) Rem(x, y Number) Number {
	PanicOnZero(ops, y)
	z := &big.Int{}
	z.Rem(x.BigInt(), y.BigInt())
	return &BigInt{B: z}
}

func (ops BigFloatOps) Rem(x, y Number) Number {
	PanicOnZero(ops, y)
	n := x.BigFloat()
	d := y.BigFloat()
	z := &big.Float{}
	i, _ := z.Quo(n, d).Int64()
	d.Mul(d, big.NewFloat(float64(i)))
	z.Sub(n, d)
	return &BigFloat{B: z}
}

func (ops RatioOps) Rem(x, y Number) Number {
	PanicOnZero(ops, y)
	n := x.Ratio()
	d := y.Ratio()
	z := big.Rat{}
	f, _ := z.Quo(n, d).Float64()
	d.Mul(d, big.NewRat(int64(f), 1))
	z.Sub(n, d)
	return RatioOrInt(&z)
}

// IsZero

func (ops IntOps) IsZero(x Number) bool {
	return x.Int().I == 0
}

func (ops DoubleOps) IsZero(x Number) bool {
	return x.Double().D == 0
}

func (ops BigIntOps) IsZero(x Number) bool {
	return x.BigInt().Sign() == 0
}

func (ops BigFloatOps) IsZero(x Number) bool {
	return x.BigFloat().Sign() == 0
}

func (ops RatioOps) IsZero(x Number) bool {
	return x.Ratio().Sign() == 0
}

// Lt

func (ops IntOps) Lt(x Number, y Number) bool {
	return x.Int().I < y.Int().I
}

func (ops DoubleOps) Lt(x Number, y Number) bool {
	return x.Double().D < y.Double().D
}

func (ops BigIntOps) Lt(x Number, y Number) bool {
	return x.BigInt().Cmp(y.BigInt()) < 0
}

func (ops BigFloatOps) Lt(x Number, y Number) bool {
	return x.BigFloat().Cmp(y.BigFloat()) < 0
}

func (ops RatioOps) Lt(x Number, y Number) bool {
	return x.Ratio().Cmp(y.Ratio()) < 0
}

// Lte

func (ops IntOps) Lte(x Number, y Number) bool {
	return x.Int().I <= y.Int().I
}

func (ops DoubleOps) Lte(x Number, y Number) bool {
	return x.Double().D <= y.Double().D
}

func (ops BigIntOps) Lte(x Number, y Number) bool {
	return x.BigInt().Cmp(y.BigInt()) <= 0
}

func (ops BigFloatOps) Lte(x Number, y Number) bool {
	return x.BigFloat().Cmp(y.BigFloat()) <= 0
}

func (ops RatioOps) Lte(x Number, y Number) bool {
	return x.Ratio().Cmp(y.Ratio()) <= 0
}

// Gt

func (ops IntOps) Gt(x Number, y Number) bool {
	return x.Int().I > y.Int().I
}

func (ops DoubleOps) Gt(x Number, y Number) bool {
	return x.Double().D > y.Double().D
}

func (ops BigIntOps) Gt(x Number, y Number) bool {
	return x.BigInt().Cmp(y.BigInt()) > 0
}

func (ops BigFloatOps) Gt(x Number, y Number) bool {
	return x.BigFloat().Cmp(y.BigFloat()) > 0
}

func (ops RatioOps) Gt(x Number, y Number) bool {
	return x.Ratio().Cmp(y.Ratio()) > 0
}

// Gte

func (ops IntOps) Gte(x Number, y Number) bool {
	return x.Int().I >= y.Int().I
}

func (ops DoubleOps) Gte(x Number, y Number) bool {
	return x.Double().D >= y.Double().D
}

func (ops BigIntOps) Gte(x Number, y Number) bool {
	return x.BigInt().Cmp(y.BigInt()) >= 0
}

func (ops BigFloatOps) Gte(x Number, y Number) bool {
	return x.BigFloat().Cmp(y.BigFloat()) >= 0
}

func (ops RatioOps) Gte(x Number, y Number) bool {
	return x.Ratio().Cmp(y.Ratio()) >= 0
}

// Eq

func (ops IntOps) Eq(x Number, y Number) bool {
	return x.Int().I == y.Int().I
}

func (ops DoubleOps) Eq(x Number, y Number) bool {
	return x.Double().D == y.Double().D
}

func (ops BigIntOps) Eq(x Number, y Number) bool {
	return x.BigInt().Cmp(y.BigInt()) == 0
}

func (ops BigFloatOps) Eq(x Number, y Number) bool {
	return x.BigFloat().Cmp(y.BigFloat()) == 0
}

func (ops RatioOps) Eq(x Number, y Number) bool {
	return x.Ratio().Cmp(y.Ratio()) == 0
}

func NumbersEq(x Number, y Number) bool {
	return GetOps(x).Combine(GetOps(y)).Eq(x, y)
}

func CompareNumbers(x Number, y Number) int {
	ops := GetOps(x).Combine(GetOps(y))
	if ops.Lt(x, y) {
		return -1
	}
	if ops.Lt(y, x) {
		return 1
	}
	return 0
}

func Max(x Number, y Number) Number {
	ops := GetOps(x).Combine(GetOps(y))
	if ops.Lt(x, y) {
		return y
	}
	return x
}

func Min(x Number, y Number) Number {
	ops := GetOps(x).Combine(GetOps(y))
	if ops.Lt(x, y) {
		return x
	}
	return y
}

func Category(x Number) int {
	switch x.(type) {
	case *BigFloat:
		return FLOATING_CATEGORY
	case Double:
		return FLOATING_CATEGORY
	case *Ratio:
		return RATIO_CATEGORY
	default:
		return INTEGER_CATEGORY
	}
}

func panicNumericError(msg string) {
	panic(msg)
}
