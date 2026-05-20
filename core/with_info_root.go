package core

import coretypes "github.com/rcarmo/go-joker/core/types"

func (x *ExInfo) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	x.Info = info
	return x
}

func (x *Fn) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	x.Info = info
	return x
}

func (x *Var) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	x.Info = info
	return x
}

func (x Nil) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	x.Info = info
	return x
}
