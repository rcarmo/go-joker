package core

import (
	"context"
	"encoding/binary"
	"math"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"

	"fmt"

	coretypes "github.com/rcarmo/go-joker/v42/core/types"
	corewasm "github.com/rcarmo/go-joker/v42/core/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// closeWasmModule closes an instantiated WASM module and reports close errors
// without changing execution semantics for best-effort cleanup paths.
func closeWasmModule(ctx context.Context, mod api.Module) {
	if err := mod.Close(ctx); err != nil {
		fmt.Fprintln(Stderr, "wasm module close error:", err)
	}
}

// wasmCompile translates IR to a WASM binary, compiles it with wazero, and
// instantiates the exported exec function. Keep this root-owned while IRProgram,
// WasmProgram, and execution slot metadata still live in package core.
func wasmCompile(prog *IRProgram) *WasmProgram {
	// Try pure-numeric path first (faster, no imports needed)
	bin := irToWasm(prog)
	// TODO: enable imports path once collection handle ABI/control-flow is fully validated.
	// if bin == nil {
	// 	bin = irToWasmWithImports(prog)
	// }
	if bin == nil {
		return nil
	}

	rt := getWasmRT()
	ctx := context.Background()

	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return nil
	}

	cfg := wazero.NewModuleConfig().WithName(corewasm.NextWasmModuleName())
	mod, err := rt.InstantiateModule(ctx, compiled, cfg)
	if err != nil {
		return nil
	}

	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		closeWasmModule(ctx, mod)
		return nil
	}
	model := runtimeExec.ProgramModel(prog)
	if model == nil {
		closeWasmModule(ctx, mod)
		return nil
	}

	wp := &WasmProgram{
		recovery:   prog,
		mod:        mod,
		execFn:     execFn,
		useFloat:   corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0),
		hasImports: !corewasm.Eligible(model.Code),
		constants:  runtimeExec.ProgramConstants(prog),
		bytes:      append([]byte(nil), bin...),
	}
	return wp
}

// Dense numeric WASM backend: opt-in checked f64 buffers, no host imports.
// Numeric compiled calls have identity, even when their Go closure code is shared.
type numericWasmCallable struct {
	Proc
	id uint64
}

func (c *numericWasmCallable) Equals(o interface{}) bool {
	x, ok := o.(*numericWasmCallable)
	return ok && c == x
}
func (c *numericWasmCallable) Hash() uint32 { return uint32(c.id ^ (c.id >> 32)) }

type WasmBuffer struct {
	coretypes.InfoHolder
	mu     sync.Mutex
	values []float64
	id     uint64
}

var wasmBufferIDs atomic.Uint64
var numericPrimitives sync.Map        // map[*Var]coretypes.Object, registered before user rebinding
func RegisterNumericPrimitive(v *Var) { numericPrimitives.LoadOrStore(v, v.Value) }
func numericPrimitiveEqual(a coretypes.Object, b interface{}) bool {
	if a == nil || b == nil {
		return false
	}
	switch x := a.(type) {
	case *Fn:
		y, ok := b.(*Fn)
		return ok && x == y
	case *Proc:
		y, ok := b.(*Proc)
		return ok && x == y
	case Proc:
		y, ok := b.(Proc)
		return ok && x.Name == y.Name && x.Package == y.Package && reflect.ValueOf(x.Fn).Pointer() == reflect.ValueOf(y.Fn).Pointer()
	}
	return false
}
func init() {
	RegisterNumericPrimitive(GLOBAL_ENV.CoreNamespace.Resolve("not"))

}

var wasmBufferType = coretypes.NewRefType("WasmBuffer", &WasmBuffer{}, nil)

func (b *WasmBuffer) ToString(bool) string {
	return fmt.Sprintf("#object[WasmBuffer %d]", len(b.values))
}
func (b *WasmBuffer) GetType() *coretypes.Type  { return wasmBufferType }
func (b *WasmBuffer) Equals(o interface{}) bool { return b == o }
func (b *WasmBuffer) Hash() uint32              { return uint32(b.id ^ (b.id >> 32)) }
func NewWasmBuffer(n int) *WasmBuffer {
	if n < 0 || n > 1<<20 {
		panic(RT.NewError("numeric buffer size must be 0..1048576"))
	}
	return &WasmBuffer{values: make([]float64, n), id: wasmBufferIDs.Add(1)}
}
func (b *WasmBuffer) Values() []float64 { return b.values } // trusted example host, not retained C memory
func WasmBufferGet(b *WasmBuffer, i int) float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if i < 0 || i >= len(b.values) {
		panic(RT.NewError("numeric buffer bounds"))
	}
	return b.values[i]
}
func WasmBufferSet(b *WasmBuffer, i int, v float64) float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if i < 0 || i >= len(b.values) {
		panic(RT.NewError("numeric buffer bounds"))
	}
	b.values[i] = v
	return v
}

type numericBinding struct{ frame, index int }
type numericKind uint8

const (
	numericUnknown numericKind = iota
	numericBottom
	numericNumber
	numericBoolean
	numericNil
	numericBuffer
)

type numericScope struct {
	slots   map[numericBinding]int
	buffers map[numericBinding]int
	kinds   map[numericBinding]numericKind
}

func numericBindingKey(b *Binding) numericBinding { return numericBinding{b.frame, b.index} }
func (e *numericEmitter) kind(x Expr) numericKind {
	switch v := x.(type) {
	case *LiteralExpr:
		switch v.obj.(type) {
		case coretypes.Int, coretypes.Double:
			return numericNumber
		case coretypes.Boolean:
			return numericBoolean
		case Nil:
			return numericNil
		}
	case *BindingExpr:
		return e.scope.kinds[numericBindingKey(v.binding)]
	case *CallExpr:
		if r, ok := v.callable.(*VarRefExpr); ok {
			switch *r.vr.name.NameKey() {
			case "<", ">", "<=", ">=", "=", "not=", "not":
				return numericBoolean
			}
			return numericNumber
		}
	case *RecurExpr:
		return numericBottom
	case *IfExpr:
		a, b := e.kind(v.positive), e.kind(v.negative)
		if a == numericBottom {
			return b
		}
		if b == numericBottom {
			return a
		}
		if a == b {
			return a
		}
	case *LetExpr:
		return e.bindingKind(v.names, v.values, v.body)
	case *LoopExpr:
		l := (*LetExpr)(v)
		return e.bindingKind(l.names, l.values, l.body)
	case *DoExpr:
		if len(v.body) > 0 {
			return e.kind(v.body[len(v.body)-1])
		}
		return numericNil
	}
	return numericUnknown
}
func (e *numericEmitter) bindingKind(names []coretypes.Symbol, values, body []Expr) numericKind {
	old := e.scope.kinds
	next := make(map[numericBinding]numericKind, len(old)+len(names))
	for k, v := range old {
		next[k] = v
	}
	e.scope.kinds = next
	e.frame++
	defer func() { e.scope.kinds = old; e.frame-- }()
	for i, v := range values {
		next[numericBinding{e.frame, i}] = e.kind(v)
	}
	if len(body) == 0 {
		return numericNil
	}
	return e.kind(body[len(body)-1])
}
func (e *numericEmitter) condition(x Expr) ([]byte, error) {
	k := e.kind(x)
	if k == numericUnknown || k == numericBuffer {
		return nil, fmt.Errorf("condition requires statically known numeric/boolean truthiness")
	}
	b, err := e.expr(x)
	if err != nil {
		return nil, err
	}
	if k == numericBoolean {
		return append(append(b, constant(0)...), 0x62), nil
	}
	b = append(b, 0x1a)
	if k == numericNil {
		return append(b, 0x41, 0), nil
	}
	return append(b, 0x41, 1), nil
}

type numericLoop struct {
	kinds []numericKind
	slots []int
	label int
}
type numericEmitter struct {
	locals int
	scope  numericScope
	labels int
	loops  []numericLoop
	nargs  int
	frame  int
	deps   map[*Var]coretypes.Object
}

func (e *numericEmitter) local() int { n := e.locals; e.locals++; return e.nargs + n }
func localOp(op byte, n int) []byte  { return corewasm.AppendULEB([]byte{op}, n) }
func constant(n float64) []byte      { return corewasm.AppendF64([]byte{0x44}, n) }
func (e *numericEmitter) body(exprs []Expr) ([]byte, error) {
	var code []byte
	for i, x := range exprs {
		b, err := e.expr(x)
		if err != nil {
			return nil, err
		}
		code = append(code, b...)
		if i < len(exprs)-1 {
			code = append(code, 0x1a)
		}
	}
	if len(exprs) == 0 {
		code = constant(0)
	}
	return code, nil
}
func (e *numericEmitter) expr(x Expr) ([]byte, error) {
	switch v := x.(type) {
	case *LiteralExpr:
		switch n := v.obj.(type) {
		case coretypes.Int:
			if int64(n.I) > 1<<53 || int64(n.I) < -(1<<53) {
				return nil, fmt.Errorf("integer literal outside exact f64 range")
			}
			return constant(float64(n.I)), nil
		case coretypes.Double:
			return constant(n.D), nil
		case Nil:
			return constant(0), nil
		case coretypes.Boolean:
			if n.B {
				return constant(1), nil
			}
			return constant(0), nil
		default:
			return nil, fmt.Errorf("non-numeric literal")
		}
	case *BindingExpr:
		n, ok := e.scope.slots[numericBindingKey(v.binding)]
		if !ok {
			return nil, fmt.Errorf("unbound numeric variable %s", v.binding.name.ToString(false))
		}
		if e.scope.kinds[numericBindingKey(v.binding)] == numericBuffer {
			return nil, fmt.Errorf("buffer cannot be used as scalar")
		}
		return localOp(0x20, n), nil
	case *DoExpr:
		return e.body(v.body)
	case *LetExpr:
		return e.bindings(v.names, v.values, v.body, false)
	case *LoopExpr:
		l := (*LetExpr)(v)
		return e.bindings(l.names, l.values, l.body, true)
	case *IfExpr:
		a, err := e.condition(v.cond)
		if err != nil {
			return nil, err
		}
		a = append(a, 0x04, 0x7c)
		e.labels++
		b, err := e.expr(v.positive)
		if err != nil {
			return nil, err
		}
		c, err := e.expr(v.negative)
		e.labels--
		if err != nil {
			return nil, err
		}
		a = append(a, b...)
		a = append(a, 0x05)
		a = append(a, c...)
		return append(a, 0x0b), nil
	case *RecurExpr:
		if len(e.loops) == 0 {
			return nil, fmt.Errorf("recur outside loop")
		}
		l := e.loops[len(e.loops)-1]
		if len(l.slots) != len(v.args) {
			return nil, fmt.Errorf("recur arity")
		}
		var code []byte
		temps := make([]int, len(v.args))
		for i, x := range v.args {
			if e.kind(x) != l.kinds[i] {
				return nil, fmt.Errorf("recur cannot change numeric binding type")
			}
			b, err := e.expr(x)
			if err != nil {
				return nil, err
			}
			temps[i] = e.local()
			code = append(code, b...)
			code = append(code, localOp(0x21, temps[i])...)
		}
		for i, n := range l.slots {
			code = append(code, localOp(0x20, temps[i])...)
			code = append(code, localOp(0x21, n)...)
		}
		code = corewasm.AppendULEB(append(code, 0x0c), e.labels-l.label)
		return append(code, constant(0)...), nil
	case *CallExpr:
		return e.call(v)
	default:
		return nil, fmt.Errorf("unsupported numeric WASM expression %T", x)
	}
}
func (e *numericEmitter) bindings(names []coretypes.Symbol, values, body []Expr, loop bool) ([]byte, error) {
	e.frame++
	defer func() { e.frame-- }()
	oldKinds := e.scope.kinds
	nextKinds := make(map[numericBinding]numericKind, len(oldKinds)+len(names))
	for k, v := range oldKinds {
		nextKinds[k] = v
	}
	e.scope.kinds = nextKinds
	old := e.scope.slots
	next := make(map[numericBinding]int, len(old)+len(names))
	for k, v := range old {
		next[k] = v
	}
	e.scope.slots = next
	oldBuffers := e.scope.buffers
	nextBuffers := make(map[numericBinding]int, len(oldBuffers))
	for k, v := range oldBuffers {
		nextBuffers[k] = v
	}
	e.scope.buffers = nextBuffers
	defer func() { e.scope.slots = old; e.scope.buffers = oldBuffers; e.scope.kinds = oldKinds }()
	var code []byte
	slots := make([]int, len(names))
	for i, x := range values {
		b, err := e.expr(x)
		if err != nil {
			return nil, err
		}
		slots[i] = e.local()
		key := numericBinding{e.frame, i}
		next[key] = slots[i]
		nextKinds[key] = e.kind(x)
		code = append(code, b...)
		code = append(code, localOp(0x21, slots[i])...)
	}
	if loop {
		code = append(code, 0x02, 0x7c, 0x03, 0x40)
		e.labels += 2
		kinds := make([]numericKind, len(names))
		for i := range names {
			kinds[i] = nextKinds[numericBinding{e.frame, i}]
		}
		e.loops = append(e.loops, numericLoop{slots: slots, kinds: kinds, label: e.labels})
	}
	b, err := e.body(body)
	if err != nil {
		return nil, err
	}
	code = append(code, b...)
	if loop {
		code = append(code, 0x0c, 0x01, 0x0b, 0x00, 0x0b)
		e.labels -= 2
		e.loops = e.loops[:len(e.loops)-1]
	}
	return code, nil
}
func (e *numericEmitter) call(v *CallExpr) ([]byte, error) {
	vr, ok := v.callable.(*VarRefExpr)
	if !ok {
		return nil, fmt.Errorf("numeric kernels do not call arbitrary functions")
	}
	name := *vr.vr.name.NameKey()
	registered, registeredOK := numericPrimitives.Load(vr.vr)
	e.deps[vr.vr] = vr.vr.Value
	if name == "buffer-get" || name == "buffer-set!" {
		if !registeredOK || !numericPrimitiveEqual(vr.vr.Value, registered) {
			return nil, fmt.Errorf("rebound buffer primitive")
		}
		return e.bufferCall(name, v.args)
	}
	if name == "not" {
		if !registeredOK || !numericPrimitiveEqual(vr.vr.Value, registered) {
			return nil, fmt.Errorf("noncanonical not primitive")
		}
		if len(v.args) != 1 {
			return nil, fmt.Errorf("not arity")
		}
		b, err := e.condition(v.args[0])
		if err != nil {
			return nil, err
		}
		return append(b, 0x45, 0xb7), nil
	}
	op := map[string]byte{"+": 0xa0, "-": 0xa1, "*": 0xa2, "/": 0xa3, "<": 0x63, ">": 0x64, "<=": 0x65, ">=": 0x66, "=": 0x61, "not=": 0x62, "sqrt": 0x9f, "abs": 0x99, "floor": 0x9c}
	instruction, ok := op[name]
	if !ok {
		return nil, fmt.Errorf("unsupported numeric operation %s", name)
	}
	// Only canonical primitives; never compile a rebound user function by spelling.
	if name == "sqrt" || name == "floor" || name == "abs" {
		if vr.vr.ns.Name.ToString(false) != "joker.math" {
			return nil, fmt.Errorf("noncanonical math operation")
		}
	} else if vr.vr.ns.Name.ToString(false) != "joker.core" {
		return nil, fmt.Errorf("noncanonical numeric operation")
	}
	if vr.vr.ns == GLOBAL_ENV.CoreNamespace {
		canonical, ok := nativeCanonicalVars[vr.vr]
		if (!ok || !numericPrimitiveEqual(vr.vr.Value, canonical)) && (!registeredOK || !numericPrimitiveEqual(vr.vr.Value, registered)) {
			return nil, fmt.Errorf("rebound numeric primitive")
		}
	} else if !registeredOK || !numericPrimitiveEqual(vr.vr.Value, registered) {
		return nil, fmt.Errorf("rebound math primitive")
	}
	for _, arg := range v.args {
		if e.kind(arg) != numericNumber {
			return nil, fmt.Errorf("numeric primitive requires numeric operands")
		}
	}
	if len(v.args) == 1 && name == "-" {
		b, err := e.expr(v.args[0])
		return append(b, 0x9a), err
	}
	unary := instruction == 0x9f || instruction == 0x99 || instruction == 0x9c
	if unary && len(v.args) != 1 || !unary && len(v.args) < 2 {
		return nil, fmt.Errorf("numeric operation arity")
	}
	if instruction >= 0x61 && instruction <= 0x66 && len(v.args) != 2 {
		return nil, fmt.Errorf("comparison requires two arguments")
	}
	var code []byte
	for i, x := range v.args {
		b, err := e.expr(x)
		if err != nil {
			return nil, err
		}
		code = append(code, b...)
		if i > 0 {
			code = append(code, instruction)
		}
	}
	if unary {
		code = append(code, instruction)
	}
	if instruction >= 0x61 && instruction <= 0x66 {
		code = append(code, 0xb7)
	}
	return code, nil
}
func (e *numericEmitter) bufferCall(name string, args []Expr) ([]byte, error) {
	if name == "buffer-get" && len(args) != 2 || name == "buffer-set!" && len(args) != 3 {
		return nil, fmt.Errorf("buffer arity")
	}
	binding, ok := args[0].(*BindingExpr)
	if !ok {
		return nil, fmt.Errorf("buffer must be declared argument")
	}
	base, ok := e.scope.buffers[numericBindingKey(binding.binding)]
	if !ok {
		return nil, fmt.Errorf("not a buffer argument")
	}
	if e.kind(args[1]) != numericNumber {
		return nil, fmt.Errorf("buffer index must be numeric")
	}
	b, err := e.expr(args[1])
	if err != nil {
		return nil, err
	}
	index := e.local()
	code := append(b, localOp(0x21, index)...)
	temp := -1
	if name == "buffer-set!" {
		if e.kind(args[2]) != numericNumber {
			return nil, fmt.Errorf("buffer value must be numeric")
		}
		value, err := e.expr(args[2])
		if err != nil {
			return nil, err
		}
		temp = e.local()
		code = append(code, value...)
		code = append(code, localOp(0x21, temp)...)
	}
	// Index must be nonnegative, integral, within declared buffer and finite.
	code = append(code, localOp(0x20, index)...)
	code = append(code, constant(0)...)
	code = append(code, 0x63, 0x04, 0x40, 0x00, 0x0b)
	code = append(code, localOp(0x20, index)...)
	code = append(code, localOp(0x20, base+1)...)
	code = append(code, 0x66, 0x04, 0x40, 0x00, 0x0b)
	code = append(code, localOp(0x20, index)...)
	code = append(code, localOp(0x20, index)...)
	code = append(code, 0x9c, 0x62, 0x04, 0x40, 0x00, 0x0b)
	code = append(code, localOp(0x20, base)...)
	code = append(code, localOp(0x20, index)...)
	code = append(code, constant(8)...)
	code = append(code, 0xa2, 0xa0, 0xaa)
	if name == "buffer-get" {
		return append(code, 0x2b, 0x03, 0x00), nil
	}
	code = append(code, localOp(0x20, temp)...)
	code = append(code, 0x39, 0x03, 0x00)
	return append(code, localOp(0x20, temp)...), nil
}

// CompileNumericWasmExported opts into f64-only dense kernels. Integer values
// must be exactly representable; unlike the existing scalar path this never
// retries through IR after a memory side effect.
func CompileNumericWasmExported(fn *Fn, bufferArgs []int) (coretypes.Object, error) {
	if fn == nil || fn.fnExpr == nil {
		return nil, fmt.Errorf("expected function")
	}
	if len(fn.fnExpr.arities) != 1 || fn.fnExpr.variadic != nil {
		return nil, fmt.Errorf("numeric kernels need one fixed arity")
	}
	arity := fn.fnExpr.arities[0]
	buffers := make(map[int]bool)
	for _, i := range bufferArgs {
		if i < 0 || i >= len(arity.args) {
			return nil, fmt.Errorf("buffer argument out of range")
		}
		if buffers[i] {
			return nil, fmt.Errorf("duplicate buffer argument")
		}
		buffers[i] = true
	}
	e := &numericEmitter{deps: map[*Var]coretypes.Object{}, scope: numericScope{slots: map[numericBinding]int{}, buffers: map[numericBinding]int{}, kinds: map[numericBinding]numericKind{}}}
	if fn.env != nil {
		e.frame = fn.env.frame + 1
	}
	for i := range arity.args {
		key := numericBinding{e.frame, i}
		e.scope.kinds[key] = numericNumber
		e.scope.slots[key] = e.nargs
		if buffers[i] {
			e.scope.buffers[key] = e.nargs
			e.scope.kinds[key] = numericBuffer
			e.nargs += 2
		} else {
			e.nargs++
		}
	}
	if len(arity.body) == 0 || e.kind(arity.body[len(arity.body)-1]) != numericNumber {
		return nil, fmt.Errorf("dense kernels must return a numeric value")
	}
	code, err := e.body(arity.body)
	if err != nil {
		return nil, err
	}
	declarations := []byte{1}
	declarations = corewasm.AppendULEB(declarations, e.locals)
	declarations = append(declarations, 0x7c)
	body := append(declarations, code...)
	body = append(body, 0x0b)
	moduleBytes := corewasm.MemoryExportModule(e.nargs, 0, body, nil)
	rt := getWasmRT()
	ctx := context.Background()
	compiled, err := rt.CompileModule(ctx, moduleBytes)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := compiled.Close(ctx); err != nil {
			fmt.Fprintln(Stderr, "numeric WASM compiled close:", err)
		}
	}()
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(corewasm.NextWasmModuleName()))
	if err != nil {
		return nil, err
	}
	function := mod.ExportedFunction("exec")
	lifetime := &struct{ live [32]byte }{}
	runtime.AddCleanup(lifetime, func(m api.Module) { closeWasmModule(context.Background(), m) }, mod)
	var mu sync.Mutex
	type argumentLocation struct {
		buffer *WasmBuffer
		start  uint32
	}
	params := make([]uint64, max(e.nargs, 1))
	locationScratch := make([]argumentLocation, 0, len(bufferArgs))
	return &numericWasmCallable{id: wasmBufferIDs.Add(1), Proc: Proc{Name: "jit-wasm-numeric", Package: "std/jit", Fn: func(args []coretypes.Object) coretypes.Object {
		defer runtime.KeepAlive(lifetime)
		CheckArity(args, len(arity.args), len(arity.args))
		for v, expected := range e.deps {
			if !numericPrimitiveEqual(v.Value, expected) {
				panic(RT.NewError("numeric WASM primitive was rebound"))
			}
		}
		mu.Lock()
		defer mu.Unlock()
		paramIndex := 0
		locations := locationScratch[:0]
		defer func() { clear(locationScratch[:cap(locationScratch)]) }()
		offset := uint64(0)
		// Kernel invocation copies isolated buffer values; same-buffer aliases share offsets.
		for i, a := range args {
			if buffers[i] {
				b, ok := a.(*WasmBuffer)
				if !ok {
					panic(RT.NewError("expected WASM numeric buffer"))
				}
				var start uint32
				seen := false
				for _, loc := range locations {
					if loc.buffer == b {
						start = loc.start
						seen = true
						break
					}
				}
				if !seen {
					if offset+uint64(len(b.values))*8 > 1<<31 {
						panic(RT.NewError("numeric buffer address limit"))
					}
					start = uint32(offset)
					locations = append(locations, argumentLocation{b, start})
					offset += uint64(len(b.values)) * 8
				}
				params[paramIndex] = math.Float64bits(float64(start))
				params[paramIndex+1] = math.Float64bits(float64(len(b.values)))
				paramIndex += 2
			} else {
				var n float64
				switch v := a.(type) {
				case coretypes.Double:
					n = v.D
				case coretypes.Int:
					if int64(v.I) > 1<<53 || int64(v.I) < -(1<<53) {
						panic(RT.NewError("numeric kernel integer outside exact f64 range"))
					}
					n = float64(v.I)
				default:
					panic(RT.NewError("numeric kernel expects Int/Double"))
				}
				params[paramIndex] = math.Float64bits(n)
				paramIndex++
			}
		}
		memory := mod.Memory()
		if offset > uint64(memory.Size()) {
			pages := uint32((offset - uint64(memory.Size()) + 65535) / 65536)
			if _, ok := memory.Grow(pages); !ok {
				panic(RT.NewError("WASM numeric memory limit"))
			}
		}
		for i := 1; i < len(locations); i++ {
			for j := i; j > 0 && locations[j].buffer.id < locations[j-1].buffer.id; j-- {
				locations[j], locations[j-1] = locations[j-1], locations[j]
			}
		}
		for _, loc := range locations {
			loc.buffer.mu.Lock()
		}
		defer func() {
			for i := len(locations) - 1; i >= 0; i-- {
				locations[i].buffer.mu.Unlock()
			}
		}()
		for _, loc := range locations {
			b, start := loc.buffer, loc.start
			data, ok := memory.Read(start, uint32(len(b.values)*8))
			if !ok {
				panic(RT.NewError("buffer copy bounds"))
			}
			for i, n := range b.values {
				binary.LittleEndian.PutUint64(data[i*8:], math.Float64bits(n))
			}
		}
		err := function.CallWithStack(ctx, params)
		for _, loc := range locations {
			b, start := loc.buffer, loc.start
			data, ok := memory.Read(start, uint32(len(b.values)*8))
			if !ok {
				panic(RT.NewError("numeric copy-back bounds"))
			}
			for i := range b.values {
				b.values[i] = math.Float64frombits(binary.LittleEndian.Uint64(data[i*8:]))
			}
		}
		if err != nil {
			panic(RT.NewError("numeric WASM trap (no replay): " + err.Error()))
		}
		return coretypes.MakeDouble(math.Float64frombits(params[0]))
	}}}, nil
}
