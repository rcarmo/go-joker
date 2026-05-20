package wasm

import (
	"context"
	"encoding/binary"
	"math"
	"sync"

	coretypes "github.com/rcarmo/go-joker/core/types"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// WasmArray represents a numeric array in WASM linear memory.
type WasmArray struct {
	mem    api.Memory
	offset uint32
	length int
	dtype  byte // 0 = f64, 1 = i64
	typ    *coretypes.Type
}

func (a *WasmArray) ToString(escape bool) string                     { return "#<wasm-array>" }
func (a *WasmArray) Equals(other interface{}) bool                   { return a == other }
func (a *WasmArray) GetInfo() *coretypes.ObjectInfo                  { return nil }
func (a *WasmArray) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return a }
func (a *WasmArray) GetType() *coretypes.Type                        { return a.typ }
func (a *WasmArray) Hash() uint32                                    { return 0 }

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

func (a *WasmArray) SetF64(i int, v float64) bool {
	if i < 0 || i >= a.length {
		return false
	}
	offset := a.offset + uint32(i*8)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
	return a.mem.Write(offset, buf[:])
}

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

func (a *WasmArray) SetI64(i int, v int64) bool {
	if i < 0 || i >= a.length {
		return false
	}
	offset := a.offset + uint32(i*8)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(v))
	return a.mem.Write(offset, buf[:])
}

func (a *WasmArray) Length() int { return a.length }

var (
	arrayMod     api.Module
	arrayModOnce sync.Once
	arrayMem     api.Memory
	nextOffset   uint32
	arrayMu      sync.Mutex
)

func initArrayModule(rt wazero.Runtime) {
	arrayModOnce.Do(func() {
		wasm := []byte{
			0x00, 0x61, 0x73, 0x6d,
			0x01, 0x00, 0x00, 0x00,
			0x05, 0x03, 0x01, 0x00, 0x40,
			0x07, 0x07, 0x01, 0x03, 0x6d, 0x65, 0x6d, 0x02, 0x00,
		}
		ctx := context.Background()
		compiled, err := rt.CompileModule(ctx, wasm)
		if err != nil {
			return
		}
		mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("joker_array_mem"))
		if err != nil {
			return
		}
		arrayMod = mod
		arrayMem = mod.Memory()
		nextOffset = 0
	})
}

func makeWasmArray(rt wazero.Runtime, size int, dtype byte, typ *coretypes.Type) *WasmArray {
	if size < 0 || size > int(^uint32(0)/8) {
		return nil
	}
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

	return &WasmArray{mem: arrayMem, offset: offset, length: size, dtype: dtype, typ: typ}
}

func MakeF64Array(rt wazero.Runtime, size int, typ *coretypes.Type) *WasmArray {
	return makeWasmArray(rt, size, 0, typ)
}

func MakeI64Array(rt wazero.Runtime, size int, typ *coretypes.Type) *WasmArray {
	return makeWasmArray(rt, size, 1, typ)
}

func MakeF64ArrayWithRuntime(getRT func() wazero.Runtime, size int, typ *coretypes.Type) *WasmArray {
	return MakeF64Array(getRT(), size, typ)
}

func MakeI64ArrayWithRuntime(getRT func() wazero.Runtime, size int, typ *coretypes.Type) *WasmArray {
	return MakeI64Array(getRT(), size, typ)
}

func ProcMakeF64Array(args []coretypes.Object, makeArr func(int) *WasmArray, nilObj coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0).Int().I
	arr := makeArr(n)
	if arr == nil {
		return nilObj
	}
	return arr
}

func ProcAgetF64(args []coretypes.Object, nilObj coretypes.Object) coretypes.Object {
	arr, ok := args[0].(*WasmArray)
	if !ok {
		return nilObj
	}
	i := coretypes.EnsureArgIsNumber(args, 1).Int().I
	return coretypes.Double{D: arr.GetF64(i)}
}

func ProcAsetF64(args []coretypes.Object, nilObj coretypes.Object) coretypes.Object {
	arr, ok := args[0].(*WasmArray)
	if !ok {
		return nilObj
	}
	i := coretypes.EnsureArgIsNumber(args, 1).Int().I
	v := coretypes.EnsureArgIsNumber(args, 2).Double().D
	if !arr.SetF64(i, v) {
		return nilObj
	}
	return arr
}

func ProcArrayLength(args []coretypes.Object, nilObj coretypes.Object) coretypes.Object {
	arr, ok := args[0].(*WasmArray)
	if !ok {
		return nilObj
	}
	return coretypes.Int{I: arr.Length()}
}
