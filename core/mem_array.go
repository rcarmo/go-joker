package core

// wasm_array.go — WASM linear memory numeric arrays.
//
// Provides fixed-size mutable numeric arrays that live in WASM linear memory.
// These bypass Go's GC entirely and give O(1) indexed access with no boxing.
//
// Usage from Joker:
//   (def a (make-f64-array 100))   ; allocate 100-element f64 array
//   (aset-f64! a 0 3.14)           ; set element
//   (aget-f64 a 0)                 ; get element → 3.14
//   (array-length a)               ; → 100
//
// The arrays are backed by WASM linear memory pages and accessed
// via host functions. This is the path for heavy vector math
// (embeddings, signal processing, matrix operations).

import (
	"context"
	"encoding/binary"
	"math"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// WasmArray represents a numeric array in WASM linear memory.
type WasmArray struct {
	mem    api.Memory
	offset uint32
	length int
	dtype  byte // 0 = f64, 1 = i64
}

func (a *WasmArray) ToString(escape bool) string   { return "#<wasm-array>" }
func (a *WasmArray) Equals(other interface{}) bool { return a == other }
func (a *WasmArray) GetInfo() *ObjectInfo          { return nil }
func (a *WasmArray) WithInfo(*ObjectInfo) Object   { return a }
func (a *WasmArray) GetType() *Type                { return TYPE.ArrayVector }
func (a *WasmArray) Hash() uint32                  { return 0 }

// GetF64 reads a float64 at index i.
func (a *WasmArray) GetF64(i int) float64 {
	if i < 0 || i >= a.length {
		return 0
	}
	offset := a.offset + uint32(i*8)
	buf, ok := a.mem.Read(offset, 8)
	if !ok {
		return 0
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(buf))
}

// SetF64 writes a float64 at index i and reports whether the write reached
// linear memory.
func (a *WasmArray) SetF64(i int, v float64) bool {
	if i < 0 || i >= a.length {
		return false
	}
	offset := a.offset + uint32(i*8)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
	return a.mem.Write(offset, buf[:])
}

// GetI64 reads an int64 at index i.
func (a *WasmArray) GetI64(i int) int64 {
	if i < 0 || i >= a.length {
		return 0
	}
	offset := a.offset + uint32(i*8)
	buf, ok := a.mem.Read(offset, 8)
	if !ok {
		return 0
	}
	return int64(binary.LittleEndian.Uint64(buf))
}

// SetI64 writes an int64 at index i and reports whether the write reached
// linear memory.
func (a *WasmArray) SetI64(i int, v int64) bool {
	if i < 0 || i >= a.length {
		return false
	}
	offset := a.offset + uint32(i*8)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(v))
	return a.mem.Write(offset, buf[:])
}

// Length returns the array size.
func (a *WasmArray) Length() int { return a.length }

// --- Array allocator ---

var (
	arrayMod     api.Module
	arrayModOnce sync.Once
	arrayMem     api.Memory
	nextOffset   uint32
	arrayMu      sync.Mutex
)

// initArrayModule creates a minimal WASM module with 1 page of linear memory.
func initArrayModule(rt wazero.Runtime) {
	arrayModOnce.Do(func() {
		// Minimal WASM module with 64 pages (4MB) of memory
		wasm := []byte{
			0x00, 0x61, 0x73, 0x6d, // magic
			0x01, 0x00, 0x00, 0x00, // version
			// Memory section: 1 memory, initial 64 pages
			0x05, 0x03, 0x01, 0x00, 0x40,
			// Export section: export memory as "mem"
			0x07, 0x07, 0x01, 0x03, 0x6d, 0x65, 0x6d, 0x02, 0x00,
		}
		ctx := context.Background()
		compiled, err := rt.CompileModule(ctx, wasm)
		if err != nil {
			return
		}
		mod, err := rt.InstantiateModule(ctx, compiled,
			wazero.NewModuleConfig().WithName("joker_array_mem"))
		if err != nil {
			return
		}
		arrayMod = mod
		arrayMem = mod.Memory()
		nextOffset = 0
	})
}

func makeWasmArray(size int, dtype byte) *WasmArray {
	if size < 0 || size > int(^uint32(0)/8) {
		return nil
	}
	rt := getWasmRT()
	initArrayModule(rt)
	if arrayMem == nil {
		return nil
	}
	byteLen := uint32(size * 8)
	zeros := make([]byte, byteLen)

	arrayMu.Lock()
	defer arrayMu.Unlock()
	offset := nextOffset
	if byteLen > ^uint32(0)-offset {
		return nil
	}
	if !arrayMem.Write(offset, zeros) {
		return nil
	}
	nextOffset += byteLen

	return &WasmArray{
		mem:    arrayMem,
		offset: offset,
		length: size,
		dtype:  dtype,
	}
}

// MakeF64Array allocates a new f64 array of the given size.
func MakeF64Array(size int) *WasmArray {
	return makeWasmArray(size, 0)
}

// MakeI64Array allocates a new i64 array of the given size.
func MakeI64Array(size int) *WasmArray {
	return makeWasmArray(size, 1)
}

// --- Joker procs for array access ---

// These can be registered as Joker procs for scripting access.

var procMakeF64Array = func(args []Object) Object {
	n := EnsureArgIsNumber(args, 0).Int().I
	arr := MakeF64Array(n)
	if arr == nil {
		return NIL
	}
	return arr
}

var procAgetF64 = func(args []Object) Object {
	arr, ok := args[0].(*WasmArray)
	if !ok {
		return NIL
	}
	i := EnsureArgIsNumber(args, 1).Int().I
	return Double{D: arr.GetF64(i)}
}

var procAsetF64 = func(args []Object) Object {
	arr, ok := args[0].(*WasmArray)
	if !ok {
		return NIL
	}
	i := EnsureArgIsNumber(args, 1).Int().I
	v := EnsureArgIsNumber(args, 2).Double().D
	if !arr.SetF64(i, v) {
		return NIL
	}
	return arr
}

var procArrayLength = func(args []Object) Object {
	arr, ok := args[0].(*WasmArray)
	if !ok {
		return NIL
	}
	return Int{I: arr.Length()}
}
