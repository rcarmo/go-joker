package core

// wasm_mem_nth.go — WASM f64 codegen with linear memory for vector nth.
//
// For loops that use f64 arithmetic + vector nth + optional helper calls,
// vector elements are copied into WASM linear memory before execution.
// The nth opcode becomes an f64.load from computed memory address.
// This eliminates all Go↔WASM boundary crossings for nth.

import (
	"context"
	"encoding/binary"
	"math"
	"reflect"
	"sync"

	corert "github.com/rcarmo/go-joker/core/runtime"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
	"github.com/tetratelabs/wazero"
)

var wasmMemNthCache sync.Map

type wasmMemNthKey struct {
	prog   *IRProgram
	helper *IRProgram
}

// wasmMemNthStaticEligible is a fast static check (no slot inspection).
func wasmMemNthStaticEligible(prog *IRProgram) bool {
	if !corert.WasmMemNthEnabled() {
		return false
	}
	model := prog.neutralModel()
	if model == nil {
		return false
	}
	code := model.Code
	pc := 0
	hasNth := false
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irDiv, irRem, irInc, irDec,
			irLt, irGte, irGt, irLte, irEq, irIsZero, irReturn, irSqrt:
			// ok
		case irNth:
			hasNth = true
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				return false
			}
		default:
			return false
		}
	}
	return hasNth
}

// Requires: f64 arithmetic, irNth on captured vectors, optional irCallSlot.
func wasmMemNthEligible(prog *IRProgram, slots []Object) bool {
	if prog == nil {
		return false
	}
	model := prog.neutralModel()
	if model == nil || len(slots) < model.NumSlots {
		return false
	}
	// Check if any slot is a Double (indicates float loop)
	hasFloat := false
	for _, s := range slots {
		if _, ok := s.(Double); ok {
			hasFloat = true
			break
		}
	}
	if !hasFloat {
		hasFloat = corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0)
	}
	if !hasFloat {
		return false
	}
	code := model.Code
	pc := 0
	hasNth := false
	nthSlots := make(map[int]bool) // which slots are used as nth collection args
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irDiv, irRem, irInc, irDec,
			irLt, irGte, irGt, irLte, irEq, irIsZero, irReturn, irSqrt:
			// ok
		case irNth:
			hasNth = true
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				return false
			}
		default:
			return false
		}
	}
	if !hasNth {
		return false
	}
	// Find which slots are loaded before nth and verify they're vectors
	pc = 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLoadSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Check if next non-load op is nth
			if pc < len(code) {
				nextOp := code[pc]
				if nextOp == irLoadSlot {
					// Pattern: load coll, load idx, nth
					nextSlot := int(code[pc+1])<<8 | int(code[pc+2])
					if pc+3 < len(code) && code[pc+3] == irNth {
						_ = nextSlot
						nthSlots[slotIdx] = true
					}
				}
			}
		case irLiteral, irStoreSlot:
			pc += 2
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				pc += 2
			}
		default:
			// single byte
		}
	}
	// Verify that nth collection slots hold ArrayVectors
	for slot := range nthSlots {
		if slot >= len(slots) {
			return false
		}
		if _, ok := slots[slot].(*ArrayVector); !ok {
			return false
		}
	}
	return true
}

type wasmMemNthCached struct {
	wp         *WasmProgram
	vecSlotIdx []int     // initSlots indices that hold vectors
	memOffsets []int     // byte offset for each vecSlotIdx
	lastVecPtr []uintptr // last-written vector pointer per slot
	paramsBuf  []uint64  // reusable params buffer
	buf8       [8]byte   // reusable byte buffer for f64 writes
}

// wasmMemNthCompileAndExec compiles and executes the loop with linear memory nth.
func wasmMemNthCompileAndExec(prog *IRProgram, slots []Object) Object {
	if !wasmMemNthEligible(prog, slots) {
		return nil
	}
	helperSlot, helperProg := findHelperForMemNth(prog, slots)

	key := wasmMemNthKey{prog: prog, helper: helperProg}
	var c *wasmMemNthCached
	if v, ok := wasmMemNthCache.Load(key); ok {
		if v == nil {
			return nil // cached failure
		}
		c = v.(*wasmMemNthCached)
	} else {
		wp := buildMemNthModule(prog, helperSlot, helperProg)
		if wp == nil {
			wasmMemNthCache.Store(key, nil)
			return nil
		}
		// Identify vector slots
		vecSlots := findVecSlots(prog, slots)
		var vecIdx []int
		var memOff []int
		offset := 0
		for _, vs := range vecSlots {
			vecIdx = append(vecIdx, vs.slot)
			memOff = append(memOff, offset)
			offset += len(vs.vec.arr) * 8
		}
		model := prog.neutralModel()
		if model == nil {
			wasmMemNthCache.Store(key, nil)
			return nil
		}
		c = &wasmMemNthCached{
			wp:         wp,
			vecSlotIdx: vecIdx,
			memOffsets: memOff,
			lastVecPtr: make([]uintptr, len(vecIdx)),
			paramsBuf:  make([]uint64, model.NumSlots),
		}
		wasmMemNthCache.Store(key, c)
	}

	// Write vector data to memory — skip if same vector pointer
	mem := c.wp.mod.ExportedMemory("memory")
	if mem == nil {
		return nil
	}
	for vi, slotIdx := range c.vecSlotIdx {
		vec := slots[slotIdx].(*ArrayVector)
		vecPtr := reflect.ValueOf(vec).Pointer()
		if vecPtr != c.lastVecPtr[vi] {
			base := c.memOffsets[vi]
			for i, obj := range vec.arr {
				var fv float64
				switch v := obj.(type) {
				case Double:
					fv = v.D
				case Int:
					fv = float64(v.I)
				default:
					return nil
				}
				binary.LittleEndian.PutUint64(c.buf8[:], math.Float64bits(fv))
				mem.Write(uint32(base+i*8), c.buf8[:])
			}
			c.lastVecPtr[vi] = vecPtr
		}
	}

	// Build params — reuse buffer
	for i, s := range slots {
		switch v := s.(type) {
		case Int:
			c.paramsBuf[i] = math.Float64bits(float64(v.I))
		case Double:
			c.paramsBuf[i] = math.Float64bits(v.D)
		default:
			// Vector slot: pass memory byte offset
			for vi, si := range c.vecSlotIdx {
				if si == i {
					c.paramsBuf[i] = math.Float64bits(float64(c.memOffsets[vi]))
					break
				}
			}
		}
	}

	ctx := context.Background()
	if err := c.wp.execFn.CallWithStack(ctx, c.paramsBuf); err != nil {
		return nil
	}
	return Double{D: math.Float64frombits(c.paramsBuf[0])}
}

type vecSlotInfo struct {
	slot int
	vec  *ArrayVector
}

func findVecSlots(prog *IRProgram, slots []Object) []vecSlotInfo {
	// Find slots loaded before irNth
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	code := model.Code
	var result []vecSlotInfo
	seen := make(map[int]bool)
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLoadSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if pc+3 < len(code) && code[pc] == irLoadSlot && code[pc+3] == irNth {
				if !seen[slotIdx] {
					if v, ok := slots[slotIdx].(*ArrayVector); ok {
						result = append(result, vecSlotInfo{slot: slotIdx, vec: v})
						seen[slotIdx] = true
					}
				}
			}
		case irLiteral, irStoreSlot:
			pc += 2
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				pc += 2
			}
		default:
		}
	}
	return result
}

func findHelperForMemNth(prog *IRProgram, slots []Object) (int, *IRProgram) {
	model := prog.neutralModel()
	if model == nil {
		return -1, nil
	}
	code := model.Code
	pc := 0
	helperSlot := -1
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irCallSlot:
			s := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			if helperSlot < 0 {
				helperSlot = s
			} else if helperSlot != s {
				return -1, nil
			}
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				pc += 2
			}
		default:
		}
	}
	if helperSlot < 0 || helperSlot >= len(slots) {
		return -1, nil
	}
	fn, ok := slots[helperSlot].(*Fn)
	if !ok {
		return -1, nil
	}
	hp := irCompileFn(fn)
	hm := hp.neutralModel()
	if hp == nil || hm == nil || !corewasm.Eligible(hm.Code) {
		return -1, nil
	}
	return helperSlot, hp
}

func buildMemNthModule(prog *IRProgram, helperSlot int, helperProg *IRProgram) *WasmProgram {
	rt := getWasmRT()
	if rt == nil {
		return nil
	}
	helperFuncIdx := -1
	helperParams := 0
	if helperProg != nil {
		helperFuncIdx = 1
		helperModel := helperProg.neutralModel()
		if helperModel == nil {
			return nil
		}
		helperParams = helperModel.NumSlots
	}
	model := prog.neutralModel()
	if model == nil {
		return nil
	}

	callerBody := buildMemNthBody(prog, helperSlot, helperFuncIdx, model.NumSlots)
	if callerBody == nil {
		return nil
	}
	var helperBody []byte
	if helperProg != nil {
		helperBody = compileWasmBodyWithHelperParams(helperProg, true, -1, -1, helperParams)
		if helperBody == nil {
			return nil
		}
	}

	bin := corewasm.MemoryExportModule(model.NumSlots, helperParams, callerBody, helperBody)
	ctx := context.Background()
	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return nil
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(corert.NextWasmModuleName()))
	if err != nil {
		return nil
	}
	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		return nil
	}
	return &WasmProgram{mod: mod, execFn: execFn, useFloat: true, hasImports: false, constants: prog.constants}
}

func buildMemNthBody(prog *IRProgram, helperSlot, helperFuncIdx, numParams int) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	var o []byte
	extra := model.NumSlots - numParams
	// Local decls: extra f64 locals + 1 i32 temp for nth address computation
	if extra > 0 {
		o = append(o, 0x02) // 2 groups
		o = corewasm.AppendULEB(o, extra)
		o = append(o, 0x7c) // f64
		o = append(o, 0x01) // 1 i32
		o = append(o, 0x7f) // i32
	} else {
		o = append(o, 0x01) // 1 group
		o = append(o, 0x01) // 1 i32
		o = append(o, 0x7f)
	}
	i32Temp := model.NumSlots // local index of i32 temp
	o = append(o, 0x02, 0x7c) // block $exit → f64
	o = append(o, 0x03, 0x40) // loop $loop → void

	code := model.Code
	pc := 0
	depth := 0

	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			c := prog.constants[idx]
			var fv float64
			switch v := c.(type) {
			case Int:
				fv = float64(v.I)
			case Double:
				fv = v.D
			default:
				return nil
			}
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, fv)
		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x20)
			o = corewasm.AppendULEB(o, idx)
		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x21)
			o = corewasm.AppendULEB(o, idx)
		case irAdd:
			o = append(o, 0xa0)
		case irSub:
			o = append(o, 0xa1)
		case irMul:
			o = append(o, 0xa2)
		case irDiv:
			o = append(o, 0xa3)
		case irSqrt:
			o = append(o, 0x9f)
		case irInc:
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, 1.0)
			o = append(o, 0xa0)
		case irDec:
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, 1.0)
			o = append(o, 0xa1)
		case irLt:
			o = append(o, 0x63) // f64.lt → i32
			o = append(o, 0xb7) // f64.convert_i32_s → f64
		case irGte:
			o = append(o, 0x65) // f64.ge → i32
			o = append(o, 0xb7)
		case irGt:
			o = append(o, 0x64) // f64.gt → i32
			o = append(o, 0xb7)
		case irLte:
			o = append(o, 0x66) // f64.le → i32
			o = append(o, 0xb7)
		case irEq:
			o = append(o, 0x61) // f64.eq → i32
			o = append(o, 0xb7)
		case irIsZero:
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, 0.0)
			o = append(o, 0x61)
			o = append(o, 0xb7)

		case irNth:
			// Stack: [base_offset_f64, idx_f64]
			// Compute address: i32(base) + i32(idx) * 8
			o = append(o, 0xaa) // i32.trunc_f64_s (idx → i32)
			o = append(o, 0x21) // local.set i32_temp
			o = corewasm.AppendULEB(o, i32Temp)
			o = append(o, 0xaa) // i32.trunc_f64_s (base → i32)
			o = append(o, 0x20) // local.get i32_temp
			o = corewasm.AppendULEB(o, i32Temp)
			o = append(o, 0x41, 0x08)       // i32.const 8
			o = append(o, 0x6c)             // i32.mul
			o = append(o, 0x6a)             // i32.add
			o = append(o, 0x2b, 0x03, 0x00) // f64.load align=3 offset=0

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			_ = nargs
			if slotIdx != helperSlot || helperFuncIdx < 0 {
				return nil
			}
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, helperFuncIdx)
		case irJumpIfNot:
			pc += 2
			// Comparison results are f64 (0.0 or 1.0), convert to i32 for if
			o = append(o, 0xaa) // i32.trunc_f64_s
			o = append(o, 0x04, 0x40)
			depth++
		case irJump:
			pc += 2
			o = append(o, 0x05)
		case irReturn:
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth+1)
			if depth > 0 && pc < len(code) && code[pc] != irJump {
				o = append(o, 0x05)
			}
		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			for i := nargs - 1; i >= 0; i-- {
				o = append(o, 0x21)
				o = corewasm.AppendULEB(o, i)
			}
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth)
			pc = len(code)
		default:
			return nil
		}
	}
	for depth > 0 {
		o = append(o, 0x0b)
		depth--
	}
	o = append(o, 0x0b)
	o = append(o, 0x44)
	o = corewasm.AppendF64(o, 0.0)
	o = append(o, 0x0b)
	o = append(o, 0x0b)
	return o
}
