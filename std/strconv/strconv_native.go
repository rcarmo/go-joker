package strconv

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math/big"

	. "github.com/rcarmo/go-joker/core"
)

func strconvIntObject(n int64) Object {
	maxNativeInt := int64(int(^uint(0) >> 1))
	minNativeInt := -maxNativeInt - 1
	if n > maxNativeInt || n < minNativeInt {
		return coretypes.MakeBigInt(big.NewInt(n))
	}
	return coretypes.MakeInt(int(n))
}
