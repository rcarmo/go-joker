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
