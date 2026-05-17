//go:build !go_spew
// +build !go_spew

package core

import coretypes "github.com/rcarmo/go-joker/core/types"

var procGoSpew = func(args []Object) (res Object) {
	return coretypes.MakeBoolean(false)
}
