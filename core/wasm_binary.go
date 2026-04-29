package core

// wasm_binary.go — low-level WASM binary format helpers.
// Produces raw WASM module bytes from sections.

import (
	"encoding/binary"
	"math"
)

// wasmModule builds a minimal WASM module binary.
type wasmModule struct {
	buf []byte
}

func newWasmModule() *wasmModule {
	m := &wasmModule{}
	m.buf = append(m.buf, 0x00, 0x61, 0x73, 0x6d) // magic: \0asm
	m.buf = append(m.buf, 0x01, 0x00, 0x00, 0x00) // version 1
	return m
}

// addTypeSection adds a func type with i64 params and result.
func (m *wasmModule) addTypeSection(numParams int) {
	m.addTypeSectionTyped(numParams, 0x7e)
}

// addTypeSectionTyped adds a func type with the given value type for all params and result.
func (m *wasmModule) addTypeSectionTyped(numParams int, valType byte) {
	var body []byte
	body = append(body, 0x01)          // 1 type entry
	body = append(body, 0x60)          // functype
	body = appendULEB(body, numParams) // param count
	for i := 0; i < numParams; i++ {
		body = append(body, valType)
	}
	body = append(body, 0x01, valType) // 1 result
	m.addSection(0x01, body)
}

// addImportSection adds the "joker" host module imports.
func (m *wasmModule) addImportSection(funcs []string, paramCounts []int) {
	var body []byte
	body = appendULEB(body, len(funcs)) // number of imports
	for i, name := range funcs {
		// module name
		modName := []byte(wasmHostModuleName)
		body = appendULEB(body, len(modName))
		body = append(body, modName...)
		// field name
		body = appendULEB(body, len(name))
		body = append(body, []byte(name)...)
		// import kind: func
		body = append(body, 0x00)
		// type index (imports come after the main type, starting at index 1)
		body = appendULEB(body, i+1)
		_ = paramCounts[i] // used for type section
	}
	m.addSection(0x02, body)
}

func (m *wasmModule) addFuncSection() {
	m.addSection(0x03, []byte{0x01, 0x00})
}

// addExportSection exports function 0 as "exec".
func (m *wasmModule) addExportSection() {
	name := []byte("exec")
	var body []byte
	body = append(body, 0x01) // 1 export
	body = appendULEB(body, len(name))
	body = append(body, name...)
	body = append(body, 0x00, 0x00) // funcidx 0
	m.addSection(0x07, body)
}

// addCodeSection adds 1 function body.
func (m *wasmModule) addCodeSection(funcBody []byte) {
	var inner []byte
	inner = append(inner, 0x01) // 1 function
	inner = appendULEB(inner, len(funcBody))
	inner = append(inner, funcBody...)
	m.addSection(0x0a, inner)
}

func (m *wasmModule) addSection(id byte, body []byte) {
	m.buf = append(m.buf, id)
	m.buf = appendULEB(m.buf, len(body))
	m.buf = append(m.buf, body...)
}

func (m *wasmModule) bytes() []byte { return m.buf }

// --- LEB128 encoding ---

func appendULEB(buf []byte, v int) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if v == 0 {
			break
		}
	}
	return buf
}

func appendSLEB(buf []byte, v int64) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			buf = append(buf, b)
			break
		}
		buf = append(buf, b|0x80)
	}
	return buf
}

func appendF64(buf []byte, v float64) []byte {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
	return append(buf, b[:]...)
}
