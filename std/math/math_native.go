package math

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math"
	"math/big"

	. "github.com/rcarmo/go-joker/core"
)

func modf(x float64) coretypes.Object {
	i, f := math.Modf(x)
	res := EmptyVector()
	res = res.Conjoin(coretypes.MakeDouble(i))
	res = res.Conjoin(coretypes.MakeDouble(f))
	return res
}

func precision(x coretypes.Number) *big.Int {
	switch n := x.(type) {
	case coretypes.Precision:
		return n.Precision()
	default:
		panic(RT.NewArgTypeError(0, x, "BigInt, BigFloat, coretypes.Int, or coretypes.Double"))
	}
}

func setPrecision(prec coretypes.Number, n *big.Float) *big.Float {
	p := prec.Int().I
	if p < 0 {
		panic(RT.NewError(fmt.Sprintf("prec must be a non-negative coretypes.Int, but is %d", p)))
	}
	return big.NewFloat(0).Copy(n).SetPrec(uint(p))
}
