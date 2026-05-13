package core

import "github.com/rcarmo/go-joker/core/internal/charcache"

var asciiCharStringObjects [128]Object

func init() {
	for i := 0; i < len(asciiCharStringObjects); i++ {
		asciiCharStringObjects[i] = String{S: charcache.String(rune(i))}
	}
}

func charToStringFast(ch rune) string {
	return charcache.String(ch)
}

func charToStringObjectFast(ch rune) Object {
	if ch >= 0 && ch < rune(len(asciiCharStringObjects)) {
		return asciiCharStringObjects[ch]
	}
	return String{S: charcache.String(ch)}
}
