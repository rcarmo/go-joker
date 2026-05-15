package collections

// BitCount returns the number of set bits in n.
func BitCount(n int) int {
	var count int
	for n != 0 {
		count++
		n &= n - 1
	}
	return count
}

// HashMask returns the 5-bit trie index for hash at shift.
func HashMask(hash uint32, shift uint) uint32 {
	return (hash >> shift) & 0x01f
}

// Bitpos returns the bitmap position for hash at shift.
func Bitpos(hash uint32, shift uint) int {
	return 1 << HashMask(hash, shift)
}
