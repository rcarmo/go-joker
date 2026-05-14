package core

import corestr "github.com/rcarmo/go-joker/core/string"

var asciiCharStringObjects = corestr.NewObjectCache(func(ch rune) Object {
	return String{S: corestr.String(ch)}
})

func charToStringFast(ch rune) string { return corestr.String(ch) }

func charToStringObjectFast(ch rune) Object {
	if obj, ok := asciiCharStringObjects.Lookup(ch); ok {
		return obj
	}
	return String{S: corestr.String(ch)}
}
