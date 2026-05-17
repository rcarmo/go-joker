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
