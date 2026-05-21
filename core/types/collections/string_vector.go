package collections

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
)

func MakeStringVector(ss []string) *ArrayVector {
	res := EmptyArrayVector()
	for _, s := range ss {
		res.Append(coretypes.MakeString(s))
	}
	return res
}
