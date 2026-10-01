package term

import (
	coretypes "github.com/rcarmo/go-joker/v42/core/types"
	corecollections "github.com/rcarmo/go-joker/v42/core/types/collections"
)

func makeTestVector(r, g, b int) coretypes.Object {
	return corecollections.NewVectorFrom(
		coretypes.MakeInt(r),
		coretypes.MakeInt(g),
		coretypes.MakeInt(b),
	)
}
