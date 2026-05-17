//go:build !go_spew
// +build !go_spew

package core

import coretypes "github.com/rcarmo/go-joker/core/types"

var procGoSpew = func(args []coretypes.Object) (res coretypes.Object) {
	return coretypes.MakeBoolean(false)
}
