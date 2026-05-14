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

func EnsureObjectIsStringable(obj Object, pattern string) String {
	switch c := obj.(type) {
	case String:
		return c
	case Char:
		return String{S: string(c.Ch)}
	default:
		panic(FailObject(c, "Stringable", pattern))
	}
}

func EnsureArgIsStringable(args []Object, index int) String {
	switch c := args[index].(type) {
	case String:
		return c
	case Char:
		return String{S: string(c.Ch)}
	default:
		panic(FailArg(c, "Stringable", index))
	}
}
