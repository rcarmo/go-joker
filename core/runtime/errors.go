package runtime

import coretypes "github.com/rcarmo/go-joker/v42/core/types"

func PanicOnErr(err error) {
	if err != nil {
		panic(coretypes.RuntimeError(err.Error()))
	}
}
