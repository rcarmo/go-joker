package runtime

import coretypes "github.com/rcarmo/go-joker/core/types"

func IsNil(obj coretypes.Object) bool {
	if obj == nil {
		return true
	}
	_, ok := obj.(Nil)
	return ok
}

func ToBool(obj coretypes.Object) bool {
	if IsNil(obj) {
		return false
	}
	switch obj := obj.(type) {
	case coretypes.Boolean:
		return obj.B
	default:
		return true
	}
}
