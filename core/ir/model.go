package ir

// Program is the package-neutral IR model. It intentionally excludes runtime
// object constants, capture keys/values, FnExpr references, native helpers, and
// execution failure caches; root core keeps those in its executable envelope
// until the object/runtime boundary is explicit.
type Program struct {
	Code            []byte
	NumSlots        int
	ConstantsLen    int
	CaptureSlotIdxs []int
	CaptureSlotSet  []bool
	HasSelf         bool
	FloatConsts     []float64
	ArityPrograms   map[int]*Program
	VariadicProgram *Program
	VariadicMinArgs int
	Analysis        *Analysis
}

func NewProgram(code []byte, numSlots, constantsLen int) *Program {
	return &Program{
		Code:         append([]byte(nil), code...),
		NumSlots:     numSlots,
		ConstantsLen: constantsLen,
	}
}

func (p *Program) WithCaptures(captureSlotIdxs []int, captureSlotSet []bool) *Program {
	if p == nil {
		return nil
	}
	p.CaptureSlotIdxs = append([]int(nil), captureSlotIdxs...)
	p.CaptureSlotSet = append([]bool(nil), captureSlotSet...)
	return p
}

func (p *Program) WithArityPrograms(programs map[int]*Program, variadic *Program, variadicMinArgs int) *Program {
	if p == nil {
		return nil
	}
	if len(programs) > 0 {
		p.ArityPrograms = make(map[int]*Program, len(programs))
		for arity, prog := range programs {
			p.ArityPrograms[arity] = prog
		}
	}
	p.VariadicProgram = variadic
	p.VariadicMinArgs = variadicMinArgs
	return p
}
