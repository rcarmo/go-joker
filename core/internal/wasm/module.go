package wasm

// Module builds a minimal WASM module binary from sections.
type Module struct {
	buf []byte
}

func NewModule() *Module {
	m := &Module{}
	m.buf = append(m.buf, 0x00, 0x61, 0x73, 0x6d) // magic: \0asm
	m.buf = append(m.buf, 0x01, 0x00, 0x00, 0x00) // version 1
	return m
}

// AddTypeSection adds a func type with i64 params and result.
func (m *Module) AddTypeSection(numParams int) { m.AddTypeSectionTyped(numParams, 0x7e) }

// AddTypeSectionTyped adds a func type with the given value type for all params and result.
func (m *Module) AddTypeSectionTyped(numParams int, valType byte) {
	var body []byte
	body = append(body, 0x01)          // 1 type entry
	body = append(body, 0x60)          // functype
	body = AppendULEB(body, numParams) // param count
	for i := 0; i < numParams; i++ {
		body = append(body, valType)
	}
	body = append(body, 0x01, valType) // 1 result
	m.AddSection(0x01, body)
}

// AddImportSection adds host module function imports.
func (m *Module) AddImportSection(hostModule string, funcs []string) {
	var body []byte
	body = AppendULEB(body, len(funcs))
	for i, name := range funcs {
		modName := []byte(hostModule)
		body = AppendULEB(body, len(modName))
		body = append(body, modName...)
		body = AppendULEB(body, len(name))
		body = append(body, []byte(name)...)
		body = append(body, 0x00) // import kind: func
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
