package runtime

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

func MakeStringVector(ss []string) *corecollections.ArrayVector {
	res := corecollections.EmptyArrayVector()
	for _, s := range ss {
		res.Append(coretypes.MakeString(s))
	}
	return res
}
