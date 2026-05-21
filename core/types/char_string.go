package types

import (
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

var asciiCharStringObjects = corestr.NewObjectCache(func(ch rune) Object {
	return String{S: corestr.CharToStringFast(ch)}
})

func CharToStringObjectFast(ch rune) Object {
	if obj, ok := asciiCharStringObjects.Lookup(ch); ok {
		return obj
	}
	return String{S: corestr.CharToStringFast(ch)}
}
