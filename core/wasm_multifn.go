package core

import (
	"context"
	"os"
	"sync"

	corewasm "github.com/rcarmo/go-joker/core/internal/wasm"
	"github.com/tetratelabs/wazero"
)

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

func wasmMultiFnMode() string {
	mode := os.Getenv("JOKER_WASM_MULTIFN")
	if mode == "" {
		return "auto"
	}
	return mode
}

func wasmMultiFnEnabled() bool {
	mode := wasmMultiFnMode()
	return mode != "0" && mode != "off" && mode != "false"
}

func wasmMultiFnForce() bool {
	mode := wasmMultiFnMode()
	return mode == "1" || mode == "force" || mode == "all"
}

func wasmGetCachedWithOneHelper(prog *IRProgram, slots []Object) *WasmProgram {
	if !wasmMultiFnEnabled() {
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
	if helperProg == nil || helperProg.hasSelf || !isWasmEligible(helperProg) {
		return 0, nil, nil, 0, false
	}
	if !isWasmEligibleWithOneHelper(prog, helperSlot) {
		return 0, nil, nil, 0, false
	}
	// Multi-function WASM: enable for both integer and float helpers.
	// Originally gated because float helpers were believed to regress,
	// but 5x median probes show no regression vs auto (within noise).
	if !wasmMultiFnForce() && helperCalls == 0 {
		return 0, nil, nil, 0, false
	}
	return helperSlot, helperFn, helperProg, helperNArgs, true
}

func isWasmEligibleWithOneHelper(prog *IRProgram, helperSlot int) bool {
	model := prog.neutralModel()
	if model == nil {
		return false
	}
	code := model.Code
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot, irNthStringASCII:
			pc += 2
		case irAdd, irSub, irMul, irRem, irInc, irDec,
			irLt, irGte, irGt, irLte, irEq, irIsZero, irReturn, irDiv, irSqrt:
			// supported
		case irCallSlot:
			slot := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			if slot != helperSlot {
				return false
			}
		case irCallSelf:
			return false
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			tgt := int(code[pc-2])<<8 | int(code[pc-1])
			if tgt != 0 {
				pc += 2
				return false
			}
		default:
			return false
		}
	}
	return true
}

func wasmCompileWithOneHelper(prog *IRProgram, helperSlot int, helperProg *IRProgram, helperParams int) *WasmProgram {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	useFloat := irProgramUsesFloat(prog) || irProgramUsesFloat(helperProg)
	callerBody := compileWasmBodyWithHelper(prog, useFloat, helperSlot, 1)
	if callerBody == nil {
		return nil
	}
	helperBody := compileWasmBodyWithHelperParams(helperProg, useFloat, -1, -1, helperParams)
	if helperBody == nil {
		return nil
	}
	bin := wasmModuleWithTwoFuncs(model.NumSlots, helperParams, useFloat, callerBody, helperBody)

	rt := getWasmRT()
	ctx := context.Background()
	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return nil
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(nextWasmModName()))
	if err != nil {
		return nil
	}
	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		return nil
	}
	return &WasmProgram{mod: mod, execFn: execFn, useFloat: useFloat, hasImports: false, constants: prog.constants}
}

func wasmModuleWithTwoFuncs(callerParams, helperParams int, useFloat bool, callerBody, helperBody []byte) []byte {
	m := newWasmModule()
	valType := corewasm.ValTypeI64
	if useFloat {
		valType = corewasm.ValTypeF64
	}
	var typeBody []byte
	typeBody = append(typeBody, 0x02)
	for _, n := range []int{callerParams, helperParams} {
		typeBody = append(typeBody, 0x60)
		typeBody = appendULEB(typeBody, n)
		for i := 0; i < n; i++ {
			typeBody = append(typeBody, valType)
		}
		typeBody = append(typeBody, 0x01, valType)
	}
	m.addSection(0x01, typeBody)
	m.addSection(0x03, []byte{0x02, 0x00, 0x01})
	m.addExportSection()
	var codeBody []byte
	codeBody = append(codeBody, 0x02)
	codeBody = appendULEB(codeBody, len(callerBody))
	codeBody = append(codeBody, callerBody...)
	codeBody = appendULEB(codeBody, len(helperBody))
	codeBody = append(codeBody, helperBody...)
	m.addSection(0x0a, codeBody)
	return m.bytes()
}
