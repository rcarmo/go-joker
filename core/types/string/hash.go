package string

func Hash(s string) uint32 {
	h := uint32(0)
	for i := 0; i < len(s) && i < 32; i++ {
		h = h*31 + uint32(s[i])
	}
	return h
}
