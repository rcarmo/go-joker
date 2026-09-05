package types

import (
	"math/big"
	"testing"
)

func TestIntMultiplyBoundaries(t *testing.T) {
	values := []int{MinInt, MinInt + 1, -100000, -2, -1, 0, 1, 2, 100000, MaxInt - 1, MaxInt}
	for _, a := range values {
		for _, b := range values {
			want := new(big.Int).Mul(big.NewInt(int64(a)), big.NewInt(int64(b)))
			got := INT_OPS.Multiply(MakeInt(a), MakeInt(b))
			if got.BigInt().Cmp(want) != 0 {
				t.Fatalf("%d * %d: got %s want %s", a, b, got.ToString(false), want)
			}
			_, isInt := got.(Int)
			fits := want.Cmp(MinIntBig) >= 0 && want.Cmp(MaxIntBig) <= 0
			if isInt != fits {
				t.Fatalf("%d * %d: incorrect promotion %T", a, b, got)
			}
		}
	}
}
