package core_test

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"testing"
)

var multiplyResult coretypes.Number

func BenchmarkIntMultiplySmall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		multiplyResult = coretypes.INT_OPS.Multiply(coretypes.MakeInt(123), coretypes.MakeInt(456))
	}
}
