package wasm

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
