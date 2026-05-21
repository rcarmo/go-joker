package runtime

import (
	"math/big"
	"os"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

func FileInfoMap(name string, info os.FileInfo, intern func(string) *string) coretypes.Map {
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(intern, "name"), coretypes.MakeString(name))
	m.Add(coretypes.MakeKeyword(intern, "size"), coretypes.IntOrBigInt(big.NewInt(info.Size())))
	m.Add(coretypes.MakeKeyword(intern, "mode"), coretypes.MakeInt(int(info.Mode())))
	m.Add(coretypes.MakeKeyword(intern, "modtime"), coretypes.MakeTime(info.ModTime()))
	m.Add(coretypes.MakeKeyword(intern, "dir?"), coretypes.MakeBoolean(info.IsDir()))
	return m
}
