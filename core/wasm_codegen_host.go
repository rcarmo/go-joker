package core

// wasm_codegen_host.go — WASM codegen with host function imports.
//
// Extends the base codegen to emit modules that import the "joker"
// host module functions for collection operations. Programs with
// collection IR opcodes (irGet, irGet3, irAssoc, irNth, irConj, etc.)
// use this path instead of the pure-numeric codegen.

// hostImport describes a host function import.
type hostImport struct {
	name      string
	numParams int // all params and results are i64
}

// standardHostImports lists the host functions in fixed order.
// Their indices in the WASM module are 0..len-1.
var standardHostImports = []hostImport{
	{"get", 2},   // (coll, key) -> result
	{"get3", 3},  // (coll, key, default) -> result
	{"assoc", 3}, // (coll, key, val) -> new_coll
	{"nth", 2},   // (coll, idx) -> result
	{"conj", 2},  // (coll, val) -> new_coll
	{"count", 1}, // (coll) -> i64
	{"first", 1}, // (coll) -> result
}

// irToWasmWithImports compiles an IR program that uses collection ops
// to a WASM module with host function imports.
func irToWasmWithImports(prog *IRProgram) []byte {
	if !isWasmWithImportsEligible(prog) {
		return nil
	}

	body := compileWasmBodyWithImports(prog)
	if body == nil {
		return nil
	}

	m := newWasmModule()

	// Type section: one type per import + one for the main fn
	// All use i64 params and i64 result
	numTypes := len(standardHostImports) + 1
	var typeBody []byte
	typeBody = appendULEB(typeBody, numTypes)
	// Import function types (index 0..6)
	for _, imp := range standardHostImports {
		typeBody = append(typeBody, 0x60) // functype
		typeBody = appendULEB(typeBody, imp.numParams)
		for j := 0; j < imp.numParams; j++ {
			typeBody = append(typeBody, 0x7e) // i64
		}
		typeBody = append(typeBody, 0x01, 0x7e) // 1 result: i64
	}
	// Main function type (index 7)
	typeBody = append(typeBody, 0x60)
	typeBody = appendULEB(typeBody, prog.numSlots)
	for i := 0; i < prog.numSlots; i++ {
		typeBody = append(typeBody, 0x7e) // i64
	}
	typeBody = append(typeBody, 0x01, 0x7e) // 1 result: i64
	m.addSection(0x01, typeBody)

	// Import section
	var importBody []byte
	importBody = appendULEB(importBody, len(standardHostImports))
	for i, imp := range standardHostImports {
		modName := []byte(wasmHostModuleName)
		importBody = appendULEB(importBody, len(modName))
		importBody = append(importBody, modName...)
		importBody = appendULEB(importBody, len(imp.name))
		importBody = append(importBody, []byte(imp.name)...)
		importBody = append(importBody, 0x00)  // import kind: func
		importBody = appendULEB(importBody, i) // type index
	}
	m.addSection(0x02, importBody)

	// Function section: 1 function with type index = len(imports)
	mainTypeIdx := len(standardHostImports)
	var funcBody []byte
	funcBody = append(funcBody, 0x01)
	funcBody = appendULEB(funcBody, mainTypeIdx)
	m.addSection(0x03, funcBody)

	// Export section: export the main function
	mainFuncIdx := len(standardHostImports) // imports are 0..6, main is 7
	name := []byte("exec")
	var exportBody []byte
	exportBody = append(exportBody, 0x01)
	exportBody = appendULEB(exportBody, len(name))
	exportBody = append(exportBody, name...)
	exportBody = append(exportBody, 0x00) // func export
	exportBody = appendULEB(exportBody, mainFuncIdx)
	m.addSection(0x07, exportBody)

	// Code section
	m.addCodeSection(body)

	return m.bytes()
}

// isWasmWithImportsEligible checks if the program can be compiled with host imports.
func isWasmWithImportsEligible(prog *IRProgram) bool {
	code := prog.code
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irDiv, irRem, irInc, irDec,
			irLt, irEq, irIsZero, irReturn, irSqrt,
			irGet, irGet3, irAssoc, irNth, irConj, irFirst, irCount:
			// all supported with imports
		case irCallSelf:
			pc += 2
		case irStr2, irBuildVec, irToTransient, irAssocBang, irToPersistent, irCallSlot:
			return false // not supported in WASM yet
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

// compileWasmBodyWithImports generates function body with host call instructions.
func compileWasmBodyWithImports(prog *IRProgram) []byte {
	var o []byte
	o = append(o, 0x00) // 0 local decls

	o = append(o, 0x02, 0x7e) // block $exit -> i64
	o = append(o, 0x03, 0x40) // loop $loop -> void

	mainFuncIdx := len(standardHostImports)
	code := prog.code
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
			case Int:
				o = append(o, 0x42)
				o = appendSLEB(o, int64(v.I))
			default:
				// Non-Int constant: use a pre-computed handle.
				// The handle value is: (1<<62) | constant_index
				// wasmExec will pre-populate the object table with these.
				handle := int64((1 << 62) | idx)
				o = append(o, 0x42)
				o = appendSLEB(o, handle)
			}

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x20)
			o = appendULEB(o, idx)

		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x21)
			o = appendULEB(o, idx)

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
			o = append(o, 0x53, 0xad)
		case irEq:
			o = append(o, 0x51, 0xad)
		case irIsZero:
			o = append(o, 0x50, 0xad)

		// Collection operations → call imported host functions
		case irGet:
			o = append(o, 0x10)  // call
			o = appendULEB(o, 0) // import index 0 = get
		case irGet3:
			o = append(o, 0x10)
			o = appendULEB(o, 1) // import index 1 = get3
		case irAssoc:
			o = append(o, 0x10)
			o = appendULEB(o, 2) // import index 2 = assoc
		case irNth:
			o = append(o, 0x10)
			o = appendULEB(o, 3) // import index 3 = nth
		case irConj:
			o = append(o, 0x10)
			o = appendULEB(o, 4) // import index 4 = conj
		case irCount:
			o = append(o, 0x10)
			o = appendULEB(o, 5) // import index 5 = count
		case irFirst:
			o = append(o, 0x10)
			o = appendULEB(o, 6) // import index 6 = first

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
			o = appendULEB(o, depth+1)
			if depth > 0 && pc < len(code) && code[pc] != irJump {
				o = append(o, 0x05)
			}

		case irCallSelf:
			pc += 2                        // skip nargs
			o = append(o, 0x10)            // call
			o = appendULEB(o, mainFuncIdx) // self = main function index

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			for i := nargs - 1; i >= 0; i-- {
				o = append(o, 0x21)
				o = appendULEB(o, i)
			}
			o = append(o, 0x0c)
			o = appendULEB(o, depth)
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
