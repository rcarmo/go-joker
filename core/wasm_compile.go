package core

import (
	"context"
	"encoding/binary"
	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"github.com/rcarmo/go-joker/core/wasm"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"math"
	"reflect"
	"sync"
)

// ---- wasm_compile.go ----
// wasm_codegen.go — translates IR bytecode to WASM function body.

var wasmFnCache sync.Map // map[*FnArityExpr]*WasmProgram

var wasmFnFail = &WasmProgram{}

// wasmGetFn retrieves or compiles a WASM program for a Fn.
func wasmGetFn(fn *Fn) *WasmProgram {
	if len(fn.fnExpr.arities) != 1 {
		return nil
	}
	arity := &fn.fnExpr.arities[0]

	if v, ok := wasmFnCache.Load(arity); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFnFail {
			return nil
		}
		return wp
	}

	// First compile to IR
	irProg := irCompileFn(fn)
	if irProg == nil {
		wasmFnCache.Store(arity, wasmFnFail)
		return nil
	}

	// Then try WASM
	wp := wasmCompile(irProg)
	if wp == nil {
		wasmFnCache.Store(arity, wasmFnFail)
		return nil
	}

	wasmFnCache.Store(arity, wp)
	return wp
}

func irToWasm(prog *IRProgram) []byte {
	model := prog.neutralModel()
	if model == nil || !corewasm.Eligible(model.Code) {
		return nil
	}
	useFloat := corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0)
	body := compileWasmBody(prog, useFloat)
	if body == nil {
		return nil
	}
	m := corewasm.NewModule()
	valType := corewasm.ValTypeI64
	if useFloat {
		valType = corewasm.ValTypeF64
	}
	m.AddTypeSectionTyped(model.NumSlots, valType)
	if prog.hasSelf {
		m.AddFuncSectionRecursive()
	} else {
		m.AddFuncSection()
	}
	m.AddExportSection()
	m.AddCodeSection(body)
	return m.Bytes()
}

// compileWasmBody generates WASM instructions.
//
// Layout:
//
//	block $exit (result i64)     ;; depth from inside if: +2
//	  loop $loop (void)          ;; depth from inside if: +1
//	    ;; body
//	    ;; irReturn → br $exit (depth = nesting + 1)
//	    ;; irRecur  → set locals, br $loop (depth = nesting)
//	  end
//	  i64.const 0  ;; unreachable
//	end
//
// For if/else: both branches end with `br` (stack-polymorphic),
// so `if void` works and no values need to flow through the if block.
func compileWasmBody(prog *IRProgram, useFloat bool) []byte {
	return compileWasmBodyWithHelper(prog, useFloat, -1, -1)
}

func compileWasmBodyWithHelper(prog *IRProgram, useFloat bool, helperSlot int, helperFuncIdx int) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	return compileWasmBodyWithHelperParams(prog, useFloat, helperSlot, helperFuncIdx, model.NumSlots)
}

func compileWasmBodyWithHelperParams(prog *IRProgram, useFloat bool, helperSlot int, helperFuncIdx int, numParams int) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	var o []byte
	valType := corewasm.ValTypeI64
	if useFloat {
		valType = corewasm.ValTypeF64
	}
	extraLocals := model.NumSlots - numParams
	if extraLocals > 0 {
		o = append(o, 0x01) // 1 local decl group
		o = corewasm.AppendULEB(o, extraLocals)
		o = append(o, valType)
	} else {
		o = append(o, 0x00) // 0 local decls
	}

	resType := valType
	if useFloat {
		resType = corewasm.ValTypeF64
	}
	o = append(o, 0x02, resType) // block $exit -> result type
	o = append(o, 0x03, 0x40)    // loop $loop -> void

	code := model.Code
	pc := 0
	depth := 0 // extra nesting from if blocks

	for pc < len(code) {
		op := code[pc]
		pc++

		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			c := prog.constants[idx]
			if useFloat {
				var fv float64
				switch v := c.(type) {
				case coretypes.Int:
					fv = float64(v.I)
				case coretypes.Double:
					fv = v.D
				default:
					return nil
				}
				o = append(o, 0x44) // f64.const
				o = corewasm.AppendF64(o, fv)
			} else {
				v, ok := c.(coretypes.Int)
				if !ok {
					return nil
				}
				o = append(o, 0x42) // i64.const
				o = corewasm.AppendSLEB(o, int64(v.I))
			}

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
			if useFloat {
				o = append(o, 0xa0)
			} else {
				o = append(o, 0x7c)
			}
		case irSub:
			if useFloat {
				o = append(o, 0xa1)
			} else {
				o = append(o, 0x7d)
			}
		case irMul:
			if useFloat {
				o = append(o, 0xa2)
			} else {
				o = append(o, 0x7e)
			}
		case irDiv:
			if useFloat {
				o = append(o, 0xa3)
			} else {
				return nil
			}
		case irSqrt:
			if useFloat {
				o = append(o, 0x9f)
			} else {
				return nil
			}
		case irRem:
			if useFloat {
				return nil
			}
			o = append(o, 0x81)
		case irInc:
			if useFloat {
				o = append(o, 0x44)
				o = corewasm.AppendF64(o, 1.0)
				o = append(o, 0xa0)
			} else {
				o = append(o, 0x42, 0x01, 0x7c)
			}
		case irDec:
			if useFloat {
				o = append(o, 0x44)
				o = corewasm.AppendF64(o, 1.0)
				o = append(o, 0xa1)
			} else {
				o = append(o, 0x42, 0x01, 0x7d)
			}
		case irLt:
			if useFloat {
				o = append(o, 0x63) // f64.lt
			} else {
				o = append(o, 0x53, 0xad) // i64.lt_s, i64.extend_i32_s
			}
		case irGte:
			if useFloat {
				o = append(o, 0x65) // f64.ge
			} else {
				o = append(o, 0x56, 0xad) // i64.ge_s, i64.extend_i32_s
			}
		case irGt:
			if useFloat {
				o = append(o, 0x64) // f64.gt
			} else {
				o = append(o, 0x55, 0xad) // i64.gt_s, i64.extend_i32_s
			}
		case irLte:
			if useFloat {
				o = append(o, 0x66) // f64.le
			} else {
				o = append(o, 0x57, 0xad) // i64.le_s, i64.extend_i32_s
			}
		case irEq:
			if useFloat {
				o = append(o, 0x61)
			} else {
				o = append(o, 0x51, 0xad)
			}
		case irIsZero:
			if useFloat {
				o = append(o, 0x44)
				o = corewasm.AppendF64(o, 0.0)
				o = append(o, 0x61)
			} else {
				o = append(o, 0x50, 0xad)
			}

		case irJumpIfNot:
			pc += 2
			if !useFloat {
				o = append(o, 0xa7) // i32.wrap_i64
			}
			// In f64 mode, comparison already left i32 on stack
			o = append(o, 0x04, 0x40) // if void
			depth++

		case irJump:
			pc += 2
			o = append(o, 0x05) // else

		case irReturn:
			// Value on stack → br to $exit (block i64)
			// Depth: depth (ifs) + 1 (loop) + 0 ($exit is the block)
			// br N targets the Nth enclosing label from current position.
			// Labels: if_0..if_{depth-1}, loop, block
			// $exit = depth + 1
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth+1)
			// If we're inside an if and no explicit else follows,
			// emit else so the false branch code has somewhere to go.
			if depth > 0 && pc < len(code) && code[pc] != irJump {
				o = append(o, 0x05) // else
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

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			_ = nargs // args already on WASM stack
			if slotIdx != helperSlot || helperFuncIdx < 0 {
				return nil
			}
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, helperFuncIdx)

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			_ = nargs                     // args already on WASM stack
			o = append(o, 0x10)           // call
			o = corewasm.AppendULEB(o, 0) // function index 0 (self)

		default:
			return nil
		}
	}

	// Close any open if blocks
	for depth > 0 {
		o = append(o, 0x0b)
		depth--
	}

	o = append(o, 0x0b) // end loop
	if useFloat {
		o = append(o, 0x44) // f64.const 0.0
		o = corewasm.AppendF64(o, 0.0)
	} else {
		o = append(o, 0x42, 0x00) // i64.const 0
	}
	o = append(o, 0x0b) // end block
	o = append(o, 0x0b) // end func
	return o
}

// ---- wasm_compile_host.go ----
// wasm_codegen_host.go — WASM codegen with host function imports.
//
// Extends the base codegen to emit modules that import the "joker"
// host module functions for collection operations. Programs with
// collection IR opcodes (irGet, irGet3, irAssoc, irNth, irConj, etc.)
// use this path instead of the pure-numeric codegen.

// standardHostImports lists the host functions in fixed order.
// Their indices in the WASM module are 0..len-1.
var standardHostImports = corewasm.StandardHostImports

// irToWasmWithImports compiles an IR program that uses collection ops
// to a WASM module with host function imports.
func irToWasmWithImports(prog *IRProgram) []byte {
	model := prog.neutralModel()
	if model == nil || !corewasm.EligibleWithImports(model.Code) {
		return nil
	}

	body := compileWasmBodyWithImports(prog)
	if body == nil {
		return nil
	}

	m := corewasm.NewModule()

	// Type section: one type per import + one for the main fn
	// All use i64 params and i64 result
	numTypes := len(standardHostImports) + 1
	var typeBody []byte
	typeBody = corewasm.AppendULEB(typeBody, numTypes)
	// Import function types (index 0..6)
	for _, imp := range standardHostImports {
		typeBody = append(typeBody, 0x60) // functype
		typeBody = corewasm.AppendULEB(typeBody, imp.NumParams)
		for j := 0; j < imp.NumParams; j++ {
			typeBody = append(typeBody, corewasm.ValTypeI64)
		}
		typeBody = append(typeBody, 0x01, corewasm.ValTypeI64)
	}
	// Main function type (index 7)
	typeBody = append(typeBody, 0x60)
	typeBody = corewasm.AppendULEB(typeBody, model.NumSlots)
	for i := 0; i < model.NumSlots; i++ {
		typeBody = append(typeBody, corewasm.ValTypeI64)
	}
	typeBody = append(typeBody, 0x01, corewasm.ValTypeI64)
	m.AddSection(0x01, typeBody)

	// Import section
	var importBody []byte
	importBody = corewasm.AppendULEB(importBody, len(standardHostImports))
	for i, imp := range standardHostImports {
		modName := []byte(wasmHostModuleName)
		importBody = corewasm.AppendULEB(importBody, len(modName))
		importBody = append(importBody, modName...)
		importBody = corewasm.AppendULEB(importBody, len(imp.Name))
		importBody = append(importBody, []byte(imp.Name)...)
		importBody = append(importBody, 0x00)           // import kind: func
		importBody = corewasm.AppendULEB(importBody, i) // type index
	}
	m.AddSection(0x02, importBody)

	// Function section: 1 function with type index = len(imports)
	mainTypeIdx := len(standardHostImports)
	var funcBody []byte
	funcBody = append(funcBody, 0x01)
	funcBody = corewasm.AppendULEB(funcBody, mainTypeIdx)
	m.AddSection(0x03, funcBody)

	// Export section: export the main function
	mainFuncIdx := len(standardHostImports) // imports are 0..6, main is 7
	name := []byte("exec")
	var exportBody []byte
	exportBody = append(exportBody, 0x01)
	exportBody = corewasm.AppendULEB(exportBody, len(name))
	exportBody = append(exportBody, name...)
	exportBody = append(exportBody, 0x00) // func export
	exportBody = corewasm.AppendULEB(exportBody, mainFuncIdx)
	m.AddSection(0x07, exportBody)

	// Code section
	m.AddCodeSection(body)

	return m.Bytes()
}

// compileWasmBodyWithImports generates function body with host call instructions.
func compileWasmBodyWithImports(prog *IRProgram) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	var o []byte
	o = append(o, 0x00) // 0 local decls

	o = append(o, 0x02, corewasm.ValTypeI64) // block $exit -> i64
	o = append(o, 0x03, 0x40)                // loop $loop -> void

	mainFuncIdx := len(standardHostImports)
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
			switch v := c.(type) {
			case coretypes.Int:
				o = append(o, 0x42)
				o = corewasm.AppendSLEB(o, int64(v.I))
			default:
				// Non-Int constant: use a pre-computed handle.
				// The handle value is: (1<<62) | constant_index
				// wasmExec will pre-populate the object table with these.
				handle := int64((1 << 62) | idx)
				o = append(o, 0x42)
				o = corewasm.AppendSLEB(o, handle)
			}

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
			o = append(o, 0x7c)
		case irSub:
			o = append(o, 0x7d)
		case irMul:
			o = append(o, 0x7e)
		case irRem:
			o = append(o, 0x81)
		case irInc:
			o = append(o, 0x42, 0x01, 0x7c)
		case irDec:
			o = append(o, 0x42, 0x01, 0x7d)
		case irLt:
			o = append(o, 0x53, 0xad) // i64.lt_s, extend
		case irGte:
			o = append(o, 0x56, 0xad) // i64.ge_s, extend
		case irGt:
			o = append(o, 0x55, 0xad) // i64.gt_s, extend
		case irLte:
			o = append(o, 0x57, 0xad) // i64.le_s, extend
		case irEq:
			o = append(o, 0x51, 0xad)
		case irIsZero:
			o = append(o, 0x50, 0xad)

		// Collection operations → call imported host functions
		case irGet:
			o = append(o, 0x10)           // call
			o = corewasm.AppendULEB(o, 0) // import index 0 = get
		case irGet3:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 1) // import index 1 = get3
		case irAssoc:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 2) // import index 2 = assoc
		case irNth:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 3) // import index 3 = nth
		case irConj:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 4) // import index 4 = conj
		case irCount:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 5) // import index 5 = count
		case irFirst:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 6) // import index 6 = first

		case irJumpIfNot:
			pc += 2
			o = append(o, 0xa7)       // i32.wrap_i64
			o = append(o, 0x04, 0x40) // if void
			depth++

		case irJump:
			pc += 2
			o = append(o, 0x05) // else

		case irReturn:
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth+1)
			if depth > 0 && pc < len(code) && code[pc] != irJump {
				o = append(o, 0x05)
			}

		case irCallSelf:
			pc += 2                                 // skip nargs
			o = append(o, 0x10)                     // call
			o = corewasm.AppendULEB(o, mainFuncIdx) // self = main function index

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			for i := nargs - 1; i >= 0; i-- {
				o = append(o, 0x21)
				o = corewasm.AppendULEB(o, i)
			}
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth)
			pc = len(code) // dead code after recur

		default:
			return nil
		}
	}

	for depth > 0 {
		o = append(o, 0x0b)
		depth--
	}
	o = append(o, 0x0b)       // end loop
	o = append(o, 0x42, 0x00) // i64.const 0
	o = append(o, 0x0b)       // end block
	o = append(o, 0x0b)       // end func
	return o
}

// ---- wasm_helper_backend.go ----
// wasm_multifn.go — experimental one-helper multi-function WASM modules.
//
// This removes the host boundary for hot loops that call a single captured
// helper function. The caller remains the exported exec function; the helper is
// emitted as a second internal WASM function and irCallSlot becomes a direct
// WASM call. This is intentionally not wired into the default eval path yet.

type wasmMultiKey struct {
	caller *IRProgram
	helper *FnArityExpr
}

var wasmMultiFnCache sync.Map    // map[wasmMultiKey]*WasmProgram
var wasmMultiFnProgFail sync.Map // map[*IRProgram]bool for no-helper/auto-rejected callers

func wasmGetCachedWithOneHelper(prog *IRProgram, slots []Object) *WasmProgram {
	if !corert.WasmMultiFnEnabled() {
		return nil
	}
	if _, failed := wasmMultiFnProgFail.Load(prog); failed {
		return nil
	}
	helperSlot, helperFn, helperProg, helperParams, ok := findSingleWasmHelper(prog, slots)
	if !ok {
		wasmMultiFnProgFail.Store(prog, true)
		return nil
	}
	key := wasmMultiKey{caller: prog, helper: &helperFn.fnExpr.arities[0]}
	if v, ok := wasmMultiFnCache.Load(key); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFail {
			return nil
		}
		return wp
	}
	wp := wasmCompileWithOneHelper(prog, helperSlot, helperProg, helperParams)
	if wp == nil {
		wasmMultiFnCache.Store(key, wasmFail)
		return nil
	}
	wasmMultiFnCache.Store(key, wp)
	return wp
}

func findSingleWasmHelper(prog *IRProgram, slots []Object) (int, *Fn, *IRProgram, int, bool) {
	model := prog.neutralModel()
	if model == nil {
		return 0, nil, nil, 0, false
	}
	code := model.Code
	pc := 0
	helperSlot := -1
	helperNArgs := -1
	helperCalls := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot, irJumpIfNot, irJump, irCallSelf, irBuildVec, irNthStringASCII:
			pc += 2
		case irCallSlot:
			slot := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			helperCalls++
			if helperSlot < 0 {
				helperSlot = slot
				helperNArgs = nargs
			} else if helperSlot != slot || helperNArgs != nargs {
				return 0, nil, nil, 0, false
			}
		case irRecur:
			pc += 4
			tgt := int(code[pc-2])<<8 | int(code[pc-1])
			if tgt != 0 {
				pc += 2
			}
		}
	}
	if helperSlot < 0 || helperSlot >= len(slots) {
		return 0, nil, nil, 0, false
	}
	helperFn, ok := slots[helperSlot].(*Fn)
	if !ok || len(helperFn.fnExpr.arities) != 1 || len(helperFn.fnExpr.arities[0].args) != helperNArgs {
		return 0, nil, nil, 0, false
	}
	helperProg := irCompileFn(helperFn)
	if helperProg == nil || helperProg.hasSelf {
		return 0, nil, nil, 0, false
	}
	helperModel := helperProg.neutralModel()
	if helperModel == nil || !corewasm.Eligible(helperModel.Code) {
		return 0, nil, nil, 0, false
	}
	if !corewasm.EligibleWithHelper(model.Code, helperSlot) {
		return 0, nil, nil, 0, false
	}
	// Multi-function WASM: enable for both integer and float helpers.
	// Originally gated because float helpers were believed to regress,
	// but 5x median probes show no regression vs auto (within noise).
	if !corert.WasmMultiFnForce() && helperCalls == 0 {
		return 0, nil, nil, 0, false
	}
	return helperSlot, helperFn, helperProg, helperNArgs, true
}

func wasmCompileWithOneHelper(prog *IRProgram, helperSlot int, helperProg *IRProgram, helperParams int) *WasmProgram {
	model := prog.neutralModel()
	if helperProg == nil {
		return nil
	}
	helperModel := helperProg.neutralModel()
	if model == nil || helperModel == nil {
		return nil
	}
	useFloat := corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0) || corewasm.UsesFloat(helperModel.Code, len(helperModel.FloatConsts) > 0)
	callerBody := compileWasmBodyWithHelper(prog, useFloat, helperSlot, 1)
	if callerBody == nil {
		return nil
	}
	helperBody := compileWasmBodyWithHelperParams(helperProg, useFloat, -1, -1, helperParams)
	if helperBody == nil {
		return nil
	}
	valType := corewasm.ValTypeI64
	if useFloat {
		valType = corewasm.ValTypeF64
	}
	bin := corewasm.TwoFuncExecModule(model.NumSlots, helperParams, valType, callerBody, helperBody)

	rt := getWasmRT()
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
	return &WasmProgram{mod: mod, execFn: execFn, useFloat: useFloat, hasImports: false, constants: prog.constants}
}

// ---- wasm_host_funcs.go ----
// wasm_host.go — Host function imports for WASM modules.
//
// Provides Joker collection operations (get, assoc, nth, conj, first, count)
// as imported host functions that WASM-compiled loops can call.
//
// Objects are passed as opaque handles (uint64 indices into a per-execution
// object table). Numeric values (Int, Double) are passed directly as i64/f64.
//
// The object table is thread-local to each wasmExec call, stored in a
// context value so host functions can access it.

// objectTable holds Joker Objects referenced by WASM code via handles.
type objectTable struct {
	objects []Object
	mu      sync.Mutex
}

// store adds an object and returns its handle.
func (t *objectTable) store(obj Object) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	idx := len(t.objects)
	t.objects = append(t.objects, obj)
	// Handles use high bit 1 to distinguish from plain i64 values
	return uint64(idx) | (1 << 62)
}

// load retrieves an object by handle.
func (t *objectTable) load(handle uint64) Object {
	idx := int(handle &^ (1 << 62))
	if idx >= 0 && idx < len(t.objects) {
		return t.objects[idx]
	}
	return NIL
}

// isHandle checks if a uint64 value is an object handle (vs plain i64).
func isHandle(v uint64) bool {
	return v&(1<<62) != 0
}

func wasmRawInt(v uint64) (int, bool) {
	i := int64(v)
	if i < int64(minInt) || i > int64(maxInt) {
		return 0, false
	}
	return int(i), true
}

func wasmRawIntObject(v uint64) Object {
	if i, ok := wasmRawInt(v); ok {
		return coretypes.Int{I: i}
	}
	return coretypes.MakeBigInt(coretypes.MakeMathBigIntFromInt64(int64(v)))
}

// contextKey for passing the object table through wazero context.
type ctxKey struct{}

func withObjectTable(ctx context.Context, t *objectTable) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

func getObjectTable(ctx context.Context) *objectTable {
	if t, ok := ctx.Value(ctxKey{}).(*objectTable); ok {
		return t
	}
	return nil
}

// wasmHostModuleName is the import module name for Joker host functions.
const wasmHostModuleName = wasm.HostModuleName

var wasmHostRegistered sync.Once

// registerWasmHost registers the "joker" host module with collection operations.
func registerWasmHost(rt wazero.Runtime) {
	wasmHostRegistered.Do(func() {
		ctx := context.Background()
		builder := rt.NewHostModuleBuilder(wasmHostModuleName)

		// joker.get(coll_handle, key_i64) -> result_i64_or_handle
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64, key uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return 0
				}
				coll := t.load(collHandle)
				var keyObj Object
				if isHandle(key) {
					keyObj = t.load(key)
				} else {
					keyObj = wasmRawIntObject(key)
				}
				if g, ok := coll.(Gettable); ok {
					ok, v := g.Get(keyObj)
					if ok {
						return objToWasm(t, v)
					}
				}
				return 0 // NIL
			}).Export("get")

		// joker.get3(coll_handle, key_i64, default_i64) -> result
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64, key uint64, def uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return def
				}
				coll := t.load(collHandle)
				var keyObj Object
				if isHandle(key) {
					keyObj = t.load(key)
				} else {
					keyObj = wasmRawIntObject(key)
				}
				if g, ok := coll.(Gettable); ok {
					ok, v := g.Get(keyObj)
					if ok {
						return objToWasm(t, v)
					}
				}
				return def
			}).Export("get3")

		// joker.assoc(coll_handle, key_i64, val_i64) -> new_coll_handle
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64, key uint64, val uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return collHandle
				}
				coll := t.load(collHandle)
				keyObj := wasmToObj(t, key)
				valObj := wasmToObj(t, val)
				if a, ok := coll.(Associative); ok {
					result := a.Assoc(keyObj, valObj)
					return t.store(result)
				}
				return collHandle
			}).Export("assoc")

		// joker.nth(coll_handle, idx_i64) -> result
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64, idx uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return 0
				}
				coll := t.load(collHandle)
				i, ok := wasmRawInt(idx)
				if !ok {
					return 0
				}
				switch c := coll.(type) {
				case *ArrayVector:
					if i >= 0 && i < len(c.arr) {
						return objToWasm(t, c.arr[i])
					}
				case Indexed:
					return objToWasm(t, c.Nth(i))
				}
				return 0
			}).Export("nth")

		// joker.conj(coll_handle, val_i64) -> new_coll_handle
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64, val uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return collHandle
				}
				coll := t.load(collHandle)
				valObj := wasmToObj(t, val)
				if c, ok := coll.(Conjable); ok {
					return t.store(c.Conj(valObj))
				}
				return collHandle
			}).Export("conj")

		// joker.first(coll_handle) -> result
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return 0
				}
				coll := t.load(collHandle)
				switch v := coll.(type) {
				case *ArrayVector:
					if len(v.arr) > 0 {
						return objToWasm(t, v.arr[0])
					}
				case Seqable:
					s := v.Seq()
					if !s.IsEmpty() {
						return objToWasm(t, s.First())
					}
				}
				return 0
			}).Export("first")

		// joker.count(coll_handle) -> i64
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return 0
				}
				coll := t.load(collHandle)
				switch v := coll.(type) {
				case coretypes.Counted:
					return uint64(v.Count())
				}
				return 0
			}).Export("count")

		builder.Instantiate(ctx)
	})
}

// objToWasm converts a Joker Object to a WASM uint64 (handle or direct value).
func objToWasm(t *objectTable, obj Object) uint64 {
	switch v := obj.(type) {
	case coretypes.Int:
		return uint64(v.I)
	case coretypes.Double:
		return math.Float64bits(v.D) | (1 << 63) // tag bit for float
	default:
		return t.store(obj)
	}
}

// wasmToObj converts a WASM uint64 back to a Joker Object.
func wasmToObj(t *objectTable, v uint64) Object {
	if isHandle(v) {
		return t.load(v)
	}
	if v&(1<<63) != 0 {
		// Float tagged value
		return coretypes.Double{D: math.Float64frombits(v &^ (1 << 63))}
	}
	return wasmRawIntObject(v)
}

// Ensure api import is used
var _ api.Module

// ---- wasm_mem_nth_backend.go ----
// wasm_mem_nth.go — WASM f64 codegen with linear memory for vector nth.
//
// For loops that use f64 arithmetic + vector nth + optional helper calls,
// vector elements are copied into WASM linear memory before execution.
// The nth opcode becomes an f64.load from computed memory address.
// This eliminates all Go↔WASM boundary crossings for nth.

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
		if _, ok := s.(coretypes.Double); ok {
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
				case coretypes.Double:
					fv = v.D
				case coretypes.Int:
					fv = float64(v.I)
				default:
					return nil
				}
				binary.LittleEndian.PutUint64(c.buf8[:], math.Float64bits(fv))
				if !mem.Write(uint32(base+i*8), c.buf8[:]) {
					return nil
				}
			}
			c.lastVecPtr[vi] = vecPtr
		}
	}

	// Build params — reuse buffer
	for i, s := range slots {
		switch v := s.(type) {
		case coretypes.Int:
			c.paramsBuf[i] = math.Float64bits(float64(v.I))
		case coretypes.Double:
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
	return coretypes.Double{D: math.Float64frombits(c.paramsBuf[0])}
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
			case coretypes.Int:
				fv = float64(v.I)
			case coretypes.Double:
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
