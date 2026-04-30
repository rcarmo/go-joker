package core

var asciiCharStrings [128]string

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
