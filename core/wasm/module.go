package wasm

import (
	"strconv"
	"sync"
)

var wasmModSeq uint64
var wasmModMu sync.Mutex

func NextWasmModuleName() string {
	wasmModMu.Lock()
	wasmModSeq++
	n := wasmModSeq
	wasmModMu.Unlock()
	return "joker_wasm_" + strconv.FormatUint(n, 10)
}

// Module builds a minimal WASM module binary from sections.
type Module struct{ buf []byte }

func NewModule() *Module {
	m := &Module{}
	m.buf = append(m.buf, 0x00, 0x61, 0x73, 0x6d)
	m.buf = append(m.buf, 0x01, 0x00, 0x00, 0x00)
	return m
}

func (m *Module) AddTypeSection(numParams int) { m.AddTypeSectionTyped(numParams, ValTypeI64) }

func (m *Module) AddTypeSectionTyped(numParams int, valType byte) {
	var body []byte
	body = append(body, 0x01, 0x60)
	body = AppendULEB(body, numParams)
	for i := 0; i < numParams; i++ {
		body = append(body, valType)
	}
	body = append(body, 0x01, valType)
	m.AddSection(0x01, body)
}

func (m *Module) AddImportSection(hostModule string, funcs []string) {
	var body []byte
	body = AppendULEB(body, len(funcs))
	for i, name := range funcs {
		modName := []byte(hostModule)
		body = AppendULEB(body, len(modName))
		body = append(body, modName...)
		body = AppendULEB(body, len(name))
		body = append(body, []byte(name)...)
		body = append(body, 0x00)
		body = AppendULEB(body, i+1)
	}
	m.AddSection(0x02, body)
}

func (m *Module) AddFuncSection()          { m.AddSection(0x03, []byte{0x01, 0x00}) }
func (m *Module) AddFuncSectionRecursive() { m.AddFuncSection() }

func (m *Module) AddExportSection() {
	name := []byte("exec")
	var body []byte
	body = append(body, 0x01)
	body = AppendULEB(body, len(name))
	body = append(body, name...)
	body = append(body, 0x00, 0x00)
	m.AddSection(0x07, body)
}

func (m *Module) AddCodeSection(funcBody []byte) {
	var inner []byte
	inner = append(inner, 0x01)
	inner = AppendULEB(inner, len(funcBody))
	inner = append(inner, funcBody...)
	m.AddSection(0x0a, inner)
}

func (m *Module) AddSection(id byte, body []byte) {
	m.buf = append(m.buf, id)
	m.buf = AppendULEB(m.buf, len(body))
	m.buf = append(m.buf, body...)
}
func (m *Module) Bytes() []byte { return append([]byte(nil), m.buf...) }

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
	execName, memName := []byte("exec"), []byte("memory")
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
