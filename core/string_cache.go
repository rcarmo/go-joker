package core

import (
	"sync"
	"unicode/utf8"
)

var asciiCharStrings [128]string

var stringRuneCountCache sync.Map // map[string]int

func init() {
	for i := 0; i < len(asciiCharStrings); i++ {
		asciiCharStrings[i] = string(rune(i))
	}
}

func charToStringFast(ch rune) string {
	if ch >= 0 && ch < rune(len(asciiCharStrings)) {
		return asciiCharStrings[ch]
	}
	return string(ch)
}

func stringRuneCountFast(s string) int {
	if v, ok := stringRuneCountCache.Load(s); ok {
		return v.(int)
	}
	count := len(s)
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			count = utf8.RuneCountInString(s)
			break
		}
	}
	stringRuneCountCache.Store(s, count)
	return count
}
