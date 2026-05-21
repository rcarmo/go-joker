package runtime

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

var asciiCharStringObjects = corestr.NewObjectCache(func(ch rune) coretypes.Object {
	return coretypes.String{S: corestr.String(ch)}
})

func CharToStringFast(ch rune) string { return corestr.String(ch) }

func CharToStringObjectFast(ch rune) coretypes.Object {
	if obj, ok := asciiCharStringObjects.Lookup(ch); ok {
		return obj
	}
	return coretypes.String{S: corestr.String(ch)}
}
