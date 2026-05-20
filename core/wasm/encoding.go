package wasm

import (
	"encoding/binary"
	"math"
)

const (
	ValTypeI32 byte = 0x7f
	ValTypeI64 byte = 0x7e
	ValTypeF32 byte = 0x7d
	ValTypeF64 byte = 0x7c

	OpEnd      byte = 0x0b
	OpI32Const byte = 0x41
	OpI64Const byte = 0x42
	OpF64Const byte = 0x44
	OpI32Add   byte = 0x6a
	OpI32Mul   byte = 0x6c
)

func AppendULEB(buf []byte, v int) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if v == 0 {
			break
		}
	}
	return buf
}

func AppendSLEB(buf []byte, v int64) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			buf = append(buf, b)
			break
		}
		buf = append(buf, b|0x80)
	}
	return buf
}

func AppendF64(buf []byte, v float64) []byte {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
	return append(buf, b[:]...)
}
