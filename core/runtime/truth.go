package runtime

import coretypes "github.com/rcarmo/go-joker/core/types"

func ToBool(obj coretypes.Object) bool {
	switch obj := obj.(type) {
	case Nil:
		return false
	case coretypes.Boolean:
		return obj.B
	default:
		return true
	}
}
