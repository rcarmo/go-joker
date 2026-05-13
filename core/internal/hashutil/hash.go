package hashutil

import (
	"hash"
	"hash/fnv"
	"unsafe"
)

func New32() hash.Hash32 {
	return fnv.New32a()
}

func Ptr(ptr uintptr) uint32 {
	h := New32()
	b := make([]byte, unsafe.Sizeof(ptr))
	b[0] = byte(ptr)
	b[1] = byte(ptr >> 8)
	b[2] = byte(ptr >> 16)
	b[3] = byte(ptr >> 24)
	if unsafe.Sizeof(ptr) == 8 {
		b[4] = byte(ptr >> 32)
		b[5] = byte(ptr >> 40)
		b[6] = byte(ptr >> 48)
		b[7] = byte(ptr >> 56)
	}
	h.Write(b)
	return h.Sum32()
}
