package string

// scanASCII is independent of the classification cache so callers and tests can
// exercise the selected kernel without cache hits hiding its execution.
var scanASCII = scanASCIIScalar

func scanASCIIScalar(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
