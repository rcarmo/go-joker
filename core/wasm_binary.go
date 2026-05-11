package core

// wasm_binary.go — low-level WASM binary format helpers.
// Produces raw WASM module bytes from sections.

import corewasm "github.com/rcarmo/go-joker/core/internal/wasm"

// wasmModule builds a minimal WASM module binary.
type wasmModule struct{ inner *corewasm.Module }

func newWasmModule() *wasmModule { return &wasmModule{inner: corewasm.NewModule()} }

// addTypeSection adds a func type with i64 params and result.
func (m *wasmModule) addTypeSection(numParams int) { m.inner.AddTypeSection(numParams) }

// addTypeSectionTyped adds a func type with the given value type for all params and result.
func (m *wasmModule) addTypeSectionTyped(numParams int, valType byte) {
	m.inner.AddTypeSectionTyped(numParams, valType)
}

// addImportSection adds the "joker" host module imports.
func (m *wasmModule) addImportSection(funcs []string, paramCounts []int) {
	_ = paramCounts // type section construction uses the counts; import section only needs function names.
	m.inner.AddImportSection(corewasm.HostModuleName, funcs)
}

// addFuncSection adds function index declarations.
func (m *wasmModule) addFuncSection() { m.inner.AddFuncSection() }

// addFuncSectionRecursive declares one function of type 0 that can call itself.
func (m *wasmModule) addFuncSectionRecursive() { m.inner.AddFuncSectionRecursive() }

// addExportSection exports function 0 as "exec".
func (m *wasmModule) addExportSection() { m.inner.AddExportSection() }

// addCodeSection adds 1 function body.
func (m *wasmModule) addCodeSection(funcBody []byte) { m.inner.AddCodeSection(funcBody) }

func (m *wasmModule) addSection(id byte, body []byte) { m.inner.AddSection(id, body) }

func (m *wasmModule) bytes() []byte { return m.inner.Bytes() }

// --- LEB128 encoding ---

func appendULEB(buf []byte, v int) []byte { return corewasm.AppendULEB(buf, v) }

func appendSLEB(buf []byte, v int64) []byte { return corewasm.AppendSLEB(buf, v) }

func appendF64(buf []byte, v float64) []byte { return corewasm.AppendF64(buf, v) }
