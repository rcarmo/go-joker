package hashutil

import (
	"encoding/binary"
	"encoding/gob"
)

func Uint32Bytes(i uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, i)
	return b
}

func Symbol(ns, name *string) uint32 {
	h := New32()
	if ns != nil {
		h.Write([]byte(*ns))
	}
	h.Write([]byte("/" + *name))
	return h.Sum32()
}

func GobEncoder(e gob.GobEncoder) uint32 {
	h := New32()
	b, err := e.GobEncode()
	if err != nil {
		panic("hashutil.GobEncoder: " + err.Error())
	}
	h.Write(b)
	return h.Sum32()
}
