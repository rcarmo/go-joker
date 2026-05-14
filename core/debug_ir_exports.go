package core

import (
	"fmt"

	corewasm "github.com/rcarmo/go-joker/core/wasm"
)

// ir_exported.go — temporary test helpers for IR debugging.

func IrCompileExported(loop *LoopExpr) *IRProgram { return irCompile(loop) }

func IrDebugCompile(loop *LoopExpr) {
	loopLet := (*LetExpr)(loop)
	fmt.Printf("loop body has %d exprs\n", len(loopLet.body))
	for i, expr := range loopLet.body {
		fmt.Printf("  body[%d]: %T\n", i, expr)
	}
	frame := guessLoopFrame(loopLet.body)
	fmt.Printf("guessed loop frame: %d\n", frame)
	c := &irCompiler{
		bindingMap: make(map[bindingKey]int),
		loopFrame:  frame,
	}
	for i := range loop.names {
		c.bindingMap[bindingKey{frame: frame, index: i}] = i
	}
	c.numSlots = len(loop.names)
	if frame < 0 {
		c.loopFrame = 1
	}
	for i, expr := range loopLet.body {
		if !c.compileExpr(expr, i == len(loopLet.body)-1) {
			fmt.Printf("  FAILED to compile body[%d]: %T\n", i, expr)
			break
		}
	}
}

func (p *IRProgram) CodeLen() int {
	model := p.neutralModel()
	return len(model.Code)
}
func (p *IRProgram) CodeBytes() []byte {
	model := p.neutralModel()
	return append([]byte(nil), model.Code...)
}
func (p *IRProgram) ConstLen() int       { return len(p.constants) }
func (p *IRProgram) Constants() []Object { return append([]Object(nil), p.constants...) }
func (p *IRProgram) NumSlots() int {
	model := p.neutralModel()
	return model.NumSlots
}
func (p *IRProgram) CaptureKeys() []bindingKey          { return p.captureKeys }
func (e *LetExpr) Body() []Expr                         { return e.body }
func IrExecExported(prog *IRProgram, s []Object) Object { return irExec(prog, s) }
func (e *LetExpr) Values() []Expr                       { return e.values }

func TestInlineInfo(expr Expr) string {
	letExpr, ok := expr.(*LetExpr)
	if !ok {
		return "not a let"
	}
	loopExpr, ok := letExpr.Body()[0].(*LoopExpr)
	if !ok {
		return "body[0] not a loop"
	}
	loopLet := (*LetExpr)(loopExpr)
	// Find the if body
	ifExpr, ok := loopLet.body[0].(*IfExpr)
	if !ok {
		return "loop body not if"
	}
	// The negative branch should have the recur with fn call
	recurExpr, ok := ifExpr.negative.(*RecurExpr)
	if !ok {
		return fmt.Sprintf("negative not recur, is %T", ifExpr.negative)
	}
	// The second recur arg should be (+ s (A i i))
	addCall, ok := recurExpr.args[1].(*CallExpr)
	if !ok {
		return fmt.Sprintf("recur arg[1] not call, is %T", recurExpr.args[1])
	}
	// The second arg of + should be (A i i)
	fnCall, ok := addCall.args[1].(*CallExpr)
	if !ok {
		return fmt.Sprintf("add arg[1] not call, is %T", addCall.args[1])
	}
	bindExpr, ok := fnCall.callable.(*BindingExpr)
	if !ok {
		return fmt.Sprintf("fn callable not binding, is %T", fnCall.callable)
	}
	if bindExpr.binding.value == nil {
		return fmt.Sprintf("binding.value is nil (frame=%d, index=%d)", bindExpr.binding.frame, bindExpr.binding.index)
	}
	return fmt.Sprintf("binding.value type: %T", bindExpr.binding.value)
}

func (fn *Fn) FnExpr() *FnExpr        { return fn.fnExpr }
func (e *FnExpr) TailRewritten() bool { return e.tailRewritten }
func (e *FnExpr) Self() *string       { return e.self.name }
func (e *LoopExpr) Values() []Expr    { return (*LetExpr)(e).values }

func WasmCompileExported(prog *IRProgram) *WasmProgram        { return wasmCompile(prog) }
func WasmExecExported(wp *WasmProgram, slots []Object) Object { return wasmExec(wp, slots) }

func IsWasmEligibleExported(prog *IRProgram) bool {
	model := prog.neutralModel()
	return model != nil && corewasm.Eligible(model.Code)
}
func IrToWasmExported(prog *IRProgram) []byte { return irToWasm(prog) }
func WasmCompileBytesExported(prog *IRProgram) []byte {
	wp := wasmCompile(prog)
	if wp == nil {
		return nil
	}
	return append([]byte(nil), wp.bytes...)
}

func IsFloatExported(prog *IRProgram) bool {
	model := prog.neutralModel()
	return model != nil && corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0)
}
func (p *IRProgram) CodeAt(i int) byte {
	model := p.neutralModel()
	return model.Code[i]
}

// --- Exports for std/jit namespace ---

func IrCompileFn(fn *Fn) *IRProgram                  { return irCompileFn(fn) }
func IrExecTyped(prog *IRProgram, s []Object) Object { return irExecTyped(prog, s) }
func IrExec(prog *IRProgram, s []Object) Object      { return irExec(prog, s) }

func (p *IRProgram) HasSelf() bool          { return p.hasSelf }
func (p *IRProgram) CaptureSlots() []Object { return p.captureSlots }
func (p *IRProgram) GetNativeHelper() func([]float64) float64 {
	if nativeHelper, ok := runtimeExec.NativeHelper(p); ok {
		return func(args []float64) float64 { return nativeHelper(args) }
	}
	return nil
}

type IRAnalysisExported struct {
	Eligible       bool
	Path           string
	HasCallSlot    bool
	HasSelfCall    bool
	UsesCollection bool
	UsesString     bool
	HasMapOps      bool
	HasAssoc       bool
	HasGenericNth  bool
}

func AnalyzeIRProgramExported(prog *IRProgram) IRAnalysisExported {
	a := AnalyzeIRProgram(prog)
	return IRAnalysisExported{
		Eligible:       irTypedEligible(a),
		Path:           a.SuggestedPath,
		HasCallSlot:    a.HasCallSlot,
		HasSelfCall:    a.HasSelfCall,
		UsesCollection: a.UsesCollection,
		UsesString:     a.UsesString,
		HasMapOps:      a.HasMapOps,
		HasAssoc:       a.HasAssoc,
		HasGenericNth:  a.HasGenericNth,
	}
}
