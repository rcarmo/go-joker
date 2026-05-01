package core

var asciiCharStrings [128]string
var asciiCharStringObjects [128]Object

func init() {
	for i := 0; i < len(asciiCharStrings); i++ {
		s := string(rune(i))
		asciiCharStrings[i] = s
		asciiCharStringObjects[i] = String{S: s}
	}
}

func charToStringFast(ch rune) string {
	if ch >= 0 && ch < rune(len(asciiCharStrings)) {
		return asciiCharStrings[ch]
	}
	return string(ch)
}

func charToStringObjectFast(ch rune) Object {
	if ch >= 0 && ch < rune(len(asciiCharStringObjects)) {
		return asciiCharStringObjects[ch]
	}
	return String{S: string(ch)}
}
