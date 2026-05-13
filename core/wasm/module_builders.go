package wasm

// TwoFuncExecModule builds a module that exports func 0 as exec and optionally
// includes a second internal helper function.
func TwoFuncExecModule(callerParams, helperParams int, valType byte, callerBody, helperBody []byte) []byte {
	m := NewModule()
	var typeBody []byte
	typeBody = append(typeBody, 0x02)
	for _, n := range []int{callerParams, helperParams} {
		typeBody = append(typeBody, 0x60)
		typeBody = AppendULEB(typeBody, n)
		for i := 0; i < n; i++ {
			typeBody = append(typeBody, valType)
		}
		typeBody = append(typeBody, 0x01, valType)
	}
	m.AddSection(0x01, typeBody)
	m.AddSection(0x03, []byte{0x02, 0x00, 0x01})
	m.AddExportSection()
	var codeBody []byte
	codeBody = append(codeBody, 0x02)
	codeBody = AppendULEB(codeBody, len(callerBody))
	codeBody = append(codeBody, callerBody...)
	codeBody = AppendULEB(codeBody, len(helperBody))
	codeBody = append(codeBody, helperBody...)
	m.AddSection(0x0a, codeBody)
	return m.Bytes()
}

// MemoryExportModule builds a float-valued module that exports exec plus a
// linear memory named "memory" and optionally includes a helper function.
func MemoryExportModule(callerParams, helperParams int, callerBody, helperBody []byte) []byte {
	m := NewModule()
	numFuncs := 1
	if helperBody != nil {
		numFuncs = 2
	}
	var typeBody []byte
	typeBody = AppendULEB(typeBody, numFuncs)
	for _, n := range []int{callerParams, helperParams}[:numFuncs] {
		typeBody = append(typeBody, 0x60)
		typeBody = AppendULEB(typeBody, n)
		for i := 0; i < n; i++ {
			typeBody = append(typeBody, ValTypeF64)
		}
		typeBody = append(typeBody, 0x01, ValTypeF64)
	}
	m.AddSection(0x01, typeBody)

	var funcBody []byte
	funcBody = AppendULEB(funcBody, numFuncs)
	for i := 0; i < numFuncs; i++ {
		funcBody = AppendULEB(funcBody, i)
	}
	m.AddSection(0x03, funcBody)
	m.AddSection(0x05, []byte{0x01, 0x00, 0x01})

	execName := []byte("exec")
	memName := []byte("memory")
	var expBody []byte
	expBody = AppendULEB(expBody, 2)
	expBody = AppendULEB(expBody, len(execName))
	expBody = append(expBody, execName...)
	expBody = append(expBody, 0x00, 0x00)
	expBody = AppendULEB(expBody, len(memName))
	expBody = append(expBody, memName...)
	expBody = append(expBody, 0x02, 0x00)
	m.AddSection(0x07, expBody)

	var codeBody []byte
	codeBody = AppendULEB(codeBody, numFuncs)
	codeBody = AppendULEB(codeBody, len(callerBody))
	codeBody = append(codeBody, callerBody...)
	if helperBody != nil {
		codeBody = AppendULEB(codeBody, len(helperBody))
		codeBody = append(codeBody, helperBody...)
	}
	m.AddSection(0x0a, codeBody)
	return m.Bytes()
}
