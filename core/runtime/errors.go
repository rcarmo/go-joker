package runtime

import coretypes "github.com/rcarmo/go-joker/core/types"

func PanicOnErr(err error) {
	if err != nil {
		panic(coretypes.RuntimeError(err.Error()))
	}
}
