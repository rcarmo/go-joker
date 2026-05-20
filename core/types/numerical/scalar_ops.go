package numerical

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	"github.com/rcarmo/go-joker/core/hashutil"
)

func NumericFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case interface{ IntValue() int }:
		return float64(n.IntValue()), true
	case interface{ DoubleValue() float64 }:
		return n.DoubleValue(), true
	default:
		return 0, false
	}
}

type HasBigFloat interface{ BigFloat() *big.Float }

func CompareNumbers(x, y HasBigFloat) int { return x.BigFloat().Cmp(y.BigFloat()) }

func CmpFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func DoubleString(d float64) string {
	if math.IsInf(d, 1) {
		return "##Inf"
	}
	if math.IsInf(d, -1) {
		return "##-Inf"
	}
	if math.IsNaN(d) {
		return "##NaN"
	}
	res := fmt.Sprintf("%g", d)
	if NeedsDecimalSuffix(res) {
		return res + ".0"
	}
	return res
}

func FloatHash64(v float64) uint32 {
	h := hashutil.New32()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, math.Float64bits(v))
	h.Write(b)
	return h.Sum32()
}

func IntHash64(v int) uint32 {
	h := hashutil.New32()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	h.Write(b)
	return h.Sum32()
}
