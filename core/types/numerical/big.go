package numerical

import (
	"encoding/gob"
	"math/big"

	"github.com/rcarmo/go-joker/core/hashutil"
)

func MakeMathBigIntFromInt(i int) *big.Int     { return MakeMathBigIntFromInt64(int64(i)) }
func MakeMathBigIntFromInt64(i int64) *big.Int { return big.NewInt(i) }
func MakeMathBigIntFromUint(b uint) *big.Int   { return MakeMathBigIntFromUint64(uint64(b)) }
func MakeMathBigIntFromUint64(b uint64) *big.Int {
	bigint := big.NewInt(0)
	bigint.SetUint64(b)
	return bigint
}

type BigFloatComparable interface{ BigFloatValue() *big.Float }

func CompareBigFloat(x, y BigFloatComparable) int {
	return x.BigFloatValue().Cmp(y.BigFloatValue())
}

func HashGob(v gob.GobEncoder) uint32 { return hashutil.GobEncoder(v) }

func NativeIntBigBounds() (min *big.Int, max *big.Int) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	return big.NewInt(int64(minInt)), big.NewInt(int64(maxInt))
}
