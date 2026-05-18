package string

var ascii [128]string

func init() {
	for i := 0; i < len(ascii); i++ {
		ascii[i] = string(rune(i))
	}
}

func String(ch rune) string {
	if ch >= 0 && ch < rune(len(ascii)) {
		return ascii[ch]
	}
	return string(ch)
}
