package runtime

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

const VERSION = "v42.8.2"

func VersionMap(intern func(string) *string) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	major, minor, incremental := corestr.ParseVersionTriplet(VERSION)
	res.Add(coretypes.MakeKeyword(intern, "major"), coretypes.Int{I: int(major)})
	res.Add(coretypes.MakeKeyword(intern, "minor"), coretypes.Int{I: int(minor)})
	res.Add(coretypes.MakeKeyword(intern, "incremental"), coretypes.Int{I: int(incremental)})
	return res
}
