package types

import "math/big"

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
