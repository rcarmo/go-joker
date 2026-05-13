package core

import corestr "github.com/rcarmo/go-joker/core/string"

var asciiCharStringObjects [128]Object

func init() {
	for i := 0; i < len(asciiCharStringObjects); i++ {
		asciiCharStringObjects[i] = String{S: corestr.String(rune(i))}
	}
}

func charToStringFast(ch rune) string {
	return corestr.String(ch)
}

func charToStringObjectFast(ch rune) Object {
	if ch >= 0 && ch < rune(len(asciiCharStringObjects)) {
		return asciiCharStringObjects[ch]
	}
	return String{S: corestr.String(ch)}
}
