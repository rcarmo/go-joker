package core

import (
	"fmt"
	coreir "github.com/rcarmo/go-joker/core/ir"
	coretypes "github.com/rcarmo/go-joker/core/types"

	corert "github.com/rcarmo/go-joker/core/runtime"
	corestr "github.com/rcarmo/go-joker/core/string"
)

// RuntimeExecutionAdapter is the narrow root-owned runtime surface that future
// extracted IR executors should target instead of reaching through all of core.
// It is intentionally small and grows only when contract tests justify a new
// operation.
type RuntimeExecutionAdapter struct{}

var runtimeExec RuntimeExecutionAdapter

func (RuntimeExecutionAdapter) Errorf(format string, args ...any) coretypes.Error {
	return RT.NewError(fmt.Sprintf(format, args...))
}

func (r RuntimeExecutionAdapter) Throw(obj Object) {
	panic(r.Errorf("%s", obj.ToString(false)))
}

func (RuntimeExecutionAdapter) Equal(a Object, b Object) bool {
	return a.Equals(b)
}

func (RuntimeExecutionAdapter) ApplyCaptureSlots(slots []Object, idxs []int, values []Object) bool {
	if len(idxs) != len(values) {
		return false
	}
	for i, obj := range values {
		idx := idxs[i]
		if idx < 0 || idx >= len(slots) {
			return false
		}
		slots[idx] = obj
	}
	return true
}

func (RuntimeExecutionAdapter) ApplyTypedCaptureSlots(slots []irValue, idxs []int, values []Object) bool {
	if len(idxs) != len(values) {
		return false
	}
	for i, obj := range values {
		idx := idxs[i]
		if idx < 0 || idx >= len(slots) {
			return false
		}
		slots[idx] = objectToIRValue(obj)
	}
	return true
}

func (r RuntimeExecutionAdapter) PrepareCallSlots(prog *IRProgram, args []Object, env *LocalEnv) []Object {
	if prog == nil || len(prog.captureKeys) == 0 {
		return args
	}
	full := make([]Object, prog.numSlots)
	copy(full, args)
	r.InstallEnvCaptures(prog, full, env)
	return full
}

func (RuntimeExecutionAdapter) InstallEnvCaptures(prog *IRProgram, slots []Object, env *LocalEnv) {
	if prog == nil {
		return
	}
	for ci, ck := range prog.captureKeys {
		if ci >= len(prog.captureSlotIdxs) {
			return
		}
		idx := prog.captureSlotIdxs[ci]
		if idx < 0 || idx >= len(slots) {
			continue
		}
		for e := env; e != nil; e = e.parent {
			if ck.index < len(e.bindings) {
				slots[idx] = e.bindings[ck.index]
				break
			}
		}
	}
}

func (RuntimeExecutionAdapter) InstallTypedEnvCaptures(prog *IRProgram, slots []irValue, env *LocalEnv) {
	if prog == nil {
		return
	}
	for ci, ck := range prog.captureKeys {
		if ci >= len(prog.captureSlotIdxs) {
			return
		}
		idx := prog.captureSlotIdxs[ci]
		if idx < 0 || idx >= len(slots) {
			continue
		}
		for e := env; e != nil; e = e.parent {
			if ck.index < len(e.bindings) {
				slots[idx] = objectToIRValue(e.bindings[ck.index])
				break
			}
		}
	}
}

func (RuntimeExecutionAdapter) MakeFn(fnExpr *FnExpr, slots []Object) Object {
	fnEnv := &LocalEnv{bindings: make([]Object, len(slots))}
	copy(fnEnv.bindings, slots)
	return &Fn{fnExpr: fnExpr, env: fnEnv}
}

func (RuntimeExecutionAdapter) CallArgs(argsSeq Object) ([]Object, bool) {
	seqable, ok := argsSeq.(Seqable)
	if !ok {
		return nil, false
	}
	seq := seqable.Seq()
	if seq == nil {
		return nil, true
	}
	return ToSlice(seq), true
}

func (RuntimeExecutionAdapter) CallObject(fnObj Object, args []Object) (Object, bool) {
	callable, ok := fnObj.(coretypes.Callable)
	if !ok {
		return nil, false
	}
	return callable.Call(args), true
}

func (adapter RuntimeExecutionAdapter) CallObjectWithSyntheticCallExpr(fnObj Object, args []Object) (Object, bool) {
	grt := currentGRT()
	prevExpr := grt.currentExpr
	grt.currentExpr = &CallExpr{}
	defer func() { grt.currentExpr = prevExpr }()
	return adapter.CallObject(fnObj, args)
}

func (RuntimeExecutionAdapter) HasMutableSlotCandidate(slots []Object) bool {
	for _, s := range slots {
		switch s.(type) {
		case *ArrayVector, *ArrayMap, *HashMap, coretypes.String:
			return true
		}
	}
	return false
}

func (RuntimeExecutionAdapter) MutableSlotObject(obj Object, escapeInfo *EscapeInfo, slot int) Object {
	if escapeInfo == nil || slot < 0 || slot >= len(escapeInfo.SafeMutableSlots) || !escapeInfo.SafeMutableSlots[slot] {
		return obj
	}
	switch v := obj.(type) {
	case *ArrayVector:
		return ToTransient(v)
	case *ArrayMap:
		return MapToTransient(v)
	case *HashMap:
		return MapToTransient(v)
	case coretypes.String:
		if !corert.IRStringBuilderDisabled() && slot < len(escapeInfo.StringPrependSlots) {
			builder := slot < len(escapeInfo.StringBuilderSlots) && escapeInfo.StringBuilderSlots[slot]
			prepend := escapeInfo.StringPrependSlots[slot]
			if corert.IRStringBuilderForce() && (builder || prepend) {
				return ToTransientString(v)
			}
			if !corert.IRStringBuilderForce() && prepend {
				return ToTransientString(v)
			}
		}
	}
	return obj
}

func (RuntimeExecutionAdapter) PersistentResult(result Object) Object {
	switch v := result.(type) {
	case *TransientVector:
		return v.ToPersistent()
	case *TransientMap:
		return v.ToPersistent()
	case *TransientString:
		return v.ToPersistent()
	default:
		return result
	}
}

func (RuntimeExecutionAdapter) Get(coll Object, key Object, def Object) Object {
	if g, ok := coll.(coretypes.Gettable); ok {
		if ok, v := g.Get(key); ok {
			return v
		}
	}
	return def
}

func (RuntimeExecutionAdapter) Assoc(coll Object, key Object, val Object) (Object, bool) {
	switch c := coll.(type) {
	case *TransientVector:
		return c.AssocInPlace(key, val), true
	case *TransientMap:
		return c.AssocInPlace(key, val), true
	case Associative:
		return c.Assoc(key, val), true
	default:
		return nil, false
	}
}

// stringNthFast returns the i-th rune of s with an ASCII-prefix fast path.
//
// Joker's string indexing is by rune index. For ASCII prefixes, byte and rune
// offsets are identical, which covers the common CLBG/gi text-processing hot
// path without changing Unicode semantics. If a non-ASCII byte appears before
// the requested index, this falls back to the Unicode-correct range walk.
func stringNthFast(s string, i int) Object {
	if i < 0 {
		panic(RT.NewError(fmt.Sprintf("Negative index: %d", i)))
	}
	if r, length, ok := corestr.NthRune(s, i); ok {
		return coretypes.Char{Ch: r}
	} else {
		panic(RT.NewError(fmt.Sprintf("Index %d exceeds string's length %d", i, length)))
	}
}

func (RuntimeExecutionAdapter) Nth(coll Object, idx int) (Object, bool) {
	switch c := coll.(type) {
	case *ArrayVector:
		if idx >= 0 && idx < len(c.arr) {
			return c.arr[idx], true
		}
	case *TransientVector:
		if idx >= 0 && idx < len(c.arr) {
			return c.arr[idx], true
		}
	case coretypes.String:
		return stringNthFast(c.S, idx), true
	case coretypes.Indexed:
		return c.Nth(idx), true
	}
	return nil, false
}

func (RuntimeExecutionAdapter) Conj(coll Object, val Object) (Object, bool) {
	switch c := coll.(type) {
	case *TransientVector:
		return c.ConjInPlace(val), true
	case coretypes.Conjable:
		return c.Conj(val), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) First(coll Object) (Object, bool) {
	switch c := coll.(type) {
	case *ArrayVector:
		if len(c.arr) > 0 {
			return c.arr[0], true
		}
		return NIL, true
	case *TransientVector:
		if len(c.arr) > 0 {
			return c.arr[0], true
		}
		return NIL, true
	case Seqable:
		s := c.Seq()
		if s == nil || s.IsEmpty() {
			return NIL, true
		}
		return s.First(), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) BuildVector(items []Object) Object {
	arr := make([]Object, len(items))
	copy(arr, items)
	return &ArrayVector{arr: arr}
}

func (RuntimeExecutionAdapter) ToTransient(coll Object) (Object, bool) {
	if av, ok := coll.(*ArrayVector); ok {
		return ToTransient(av), true
	}
	return nil, false
}

func (RuntimeExecutionAdapter) AssocBang(coll Object, key Object, val Object) (Object, bool) {
	switch c := coll.(type) {
	case *TransientVector:
		return c.AssocInPlace(key, val), true
	case *TransientMap:
		return c.AssocInPlace(key, val), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) ToPersistent(coll Object) (Object, bool) {
	switch c := coll.(type) {
	case *TransientVector:
		return c.ToPersistent(), true
	case *TransientMap:
		return c.ToPersistent(), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) Str1(obj Object) Object {
	switch v := obj.(type) {
	case Nil:
		return coretypes.String{S: ""}
	case coretypes.String:
		return v
	case coretypes.Char:
		return charToStringObjectFast(v.Ch)
	default:
		return coretypes.String{S: obj.ToString(false)}
	}
}

func (RuntimeExecutionAdapter) Str2(a Object, b Object) Object {
	switch av := a.(type) {
	case *TransientString:
		switch bv := b.(type) {
		case coretypes.Char:
			return av.AppendChar(bv.Ch)
		case coretypes.String:
			return av.AppendString(bv.S)
		default:
			return av.AppendString(b.ToString(false))
		}
	case coretypes.String:
		switch bv := b.(type) {
		case coretypes.Char:
			return coretypes.String{S: av.S + charToStringFast(bv.Ch)}
		case coretypes.String:
			return coretypes.String{S: av.S + bv.S}
		case *TransientString:
			return bv.PrependString(av.S)
		default:
			return coretypes.String{S: av.S + b.ToString(false)}
		}
	case coretypes.Char:
		if bv, ok := b.(*TransientString); ok {
			return bv.PrependChar(av.Ch)
		}
		return coretypes.String{S: charToStringFast(av.Ch) + b.ToString(false)}
	default:
		return coretypes.String{S: a.ToString(false) + b.ToString(false)}
	}
}

func (RuntimeExecutionAdapter) Count(obj Object) (int, bool) {
	switch v := obj.(type) {
	case *TransientString:
		return v.Count(), true
	case coretypes.Counted:
		return v.Count(), true
	default:
		return 0, false
	}
}

func (adapter RuntimeExecutionAdapter) NthASCIIStringConst(prog *IRProgram, constIdx int, idx int) (Object, bool) {
	constant, ok := adapter.ProgramConstant(prog, constIdx)
	if !ok {
		return nil, false
	}
	s, ok := constant.(coretypes.String)
	if !ok || idx < 0 || idx >= len(s.S) {
		return nil, false
	}
	return coretypes.Char{Ch: rune(s.S[idx])}, true
}

func (RuntimeExecutionAdapter) CursorChar(obj Object) (Object, bool) {
	cur, ok := obj.(*StringCursor)
	if !ok {
		return nil, false
	}
	if r := cur.Char(); r >= 0 {
		return coretypes.Char{Ch: r}, true
	}
	return NIL, true
}

func (RuntimeExecutionAdapter) CursorNext(obj Object) (Object, bool) {
	cur, ok := obj.(*StringCursor)
	if !ok {
		return nil, false
	}
	return cur.Next(), true
}

func (RuntimeExecutionAdapter) CursorDone(obj Object) (Object, bool) {
	cur, ok := obj.(*StringCursor)
	if !ok {
		return nil, false
	}
	return coretypes.Boolean{B: cur.Done()}, true
}

func (RuntimeExecutionAdapter) MarkTypedExecutionFailed(prog *IRProgram) {
	if prog != nil {
		prog.typedFailed = true
	}
}

func (RuntimeExecutionAdapter) MarkBoxedExecutionFailed(prog *IRProgram) {
	if prog != nil {
		prog.execFailed = true
	}
}

func (RuntimeExecutionAdapter) ProgramNumSlots(prog *IRProgram) int {
	if prog == nil {
		return 0
	}
	return prog.numSlots
}

func (RuntimeExecutionAdapter) ProgramCode(prog *IRProgram) []byte {
	if prog == nil {
		return nil
	}
	return prog.code
}

func (RuntimeExecutionAdapter) ProgramModel(prog *IRProgram) *coreir.Program {
	if prog == nil {
		return nil
	}
	return prog.neutralModel()
}

func (RuntimeExecutionAdapter) ProgramConstant(prog *IRProgram, idx int) (Object, bool) {
	if prog == nil || idx < 0 || idx >= len(prog.constants) {
		return nil, false
	}
	return prog.constants[idx], true
}

func (RuntimeExecutionAdapter) ProgramConstants(prog *IRProgram) []Object {
	if prog == nil {
		return nil
	}
	return prog.constants
}

func (RuntimeExecutionAdapter) ProgramFnExpr(prog *IRProgram, idx int) (*FnExpr, bool) {
	if prog == nil || idx < 0 || idx >= len(prog.fnExprs) {
		return nil, false
	}
	return prog.fnExprs[idx], true
}

func (RuntimeExecutionAdapter) FnProgram(fnObj Object) (*IRProgram, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	if fn.irProg != nil {
		if fn.irProg == irCompileFailed {
			return nil, false
		}
		return fn.irProg, true
	}
	prog := irGetFnProg(fn)
	return prog, prog != nil
}

func (RuntimeExecutionAdapter) CompileFnProgram(fnObj Object) (*IRProgram, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	prog := irCompileFn(fn)
	return prog, prog != nil
}

func (RuntimeExecutionAdapter) FnWasmExec(fnObj Object, args []Object) (Object, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	wp := wasmGetFn(fn)
	if wp == nil {
		return nil, false
	}
	result := wasmExec(wp, args)
	return result, result != nil
}

func (adapter RuntimeExecutionAdapter) FnCallSlots(fnObj Object, prog *IRProgram, args []Object) ([]Object, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	return adapter.PrepareCallSlots(prog, args, fn.env), true
}

func (adapter RuntimeExecutionAdapter) InstallFnTypedEnvCaptures(fnObj Object, prog *IRProgram, slots []irValue) bool {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return false
	}
	adapter.InstallTypedEnvCaptures(prog, slots, fn.env)
	return true
}

func (RuntimeExecutionAdapter) ObjectsFromTypedValues(values []irValue, buf []Object) []Object {
	var out []Object
	if len(values) <= len(buf) {
		out = buf[:len(values)]
	} else {
		out = make([]Object, len(values))
	}
	for i, v := range values {
		out[i] = v.object()
	}
	return out
}

func (adapter RuntimeExecutionAdapter) DispatchArityProgram(prog *IRProgram, nargs int) *IRProgram {
	if prog == nil {
		return nil
	}
	if prog.arityPrograms == nil {
		if prog.variadicMinArgs > 0 && nargs < prog.variadicMinArgs {
			return nil
		}
		return prog
	}
	if sub, ok := prog.arityPrograms[nargs]; ok {
		return sub
	}
	if prog.variadicProg != nil && nargs >= prog.variadicMinArgs {
		return prog.variadicProg
	}
	return nil
}

func (RuntimeExecutionAdapter) ProgramHasCaptureSlots(prog *IRProgram) bool {
	return prog != nil && len(prog.captureSlots) > 0
}

func (RuntimeExecutionAdapter) ProgramEscapeInfo(prog *IRProgram) *EscapeInfo {
	if prog == nil {
		return nil
	}
	if prog.escapeInfo == nil {
		prog.escapeInfo = analyzeEscapes(prog)
	}
	return prog.escapeInfo
}

func (RuntimeExecutionAdapter) ProgramAnalysis(prog *IRProgram) coreir.Analysis {
	return AnalyzeIRProgram(prog)
}

func (adapter RuntimeExecutionAdapter) ApplyProgramCaptureSlots(prog *IRProgram, slots []Object) bool {
	if prog == nil {
		return false
	}
	return adapter.ApplyCaptureSlots(slots, prog.captureSlotIdxs, prog.captureSlots)
}

func (adapter RuntimeExecutionAdapter) ApplyProgramTypedCaptureSlots(prog *IRProgram, slots []irValue) bool {
	if prog == nil {
		return false
	}
	return adapter.ApplyTypedCaptureSlots(slots, prog.captureSlotIdxs, prog.captureSlots)
}

func (adapter RuntimeExecutionAdapter) ClearTypedNonCaptureSlots(prog *IRProgram, slots []irValue, keepArgs int) bool {
	if keepArgs < 0 || keepArgs > len(slots) {
		return false
	}
	if prog != nil && prog.captureSlotSet != nil {
		if len(prog.captureSlotSet) < len(slots) {
			return false
		}
		for i := keepArgs; i < len(slots); i++ {
			if !prog.captureSlotSet[i] {
				slots[i] = irValue{}
			}
		}
		return true
	}
	for i := keepArgs; i < len(slots); i++ {
		slots[i] = irValue{}
	}
	if prog == nil || len(prog.captureSlots) == 0 {
		return true
	}
	return adapter.ApplyProgramTypedCaptureSlots(prog, slots)
}

func (RuntimeExecutionAdapter) ProgramCaptureSlots(prog *IRProgram) ([]int, []Object) {
	if prog == nil {
		return nil, nil
	}
	return prog.captureSlotIdxs, prog.captureSlots
}

func (RuntimeExecutionAdapter) CanExecuteIR(prog *IRProgram) bool {
	return prog != nil && !prog.execFailed
}

func (RuntimeExecutionAdapter) CanExecuteTypedIR(prog *IRProgram) bool {
	return prog != nil && !prog.typedFailed && !prog.execFailed
}

func (RuntimeExecutionAdapter) HasNativeHelper(prog *IRProgram) bool {
	return prog != nil && prog.nativeHelper != nil
}

func (RuntimeExecutionAdapter) NativeHelper(prog *IRProgram) (nativeF64Fn, bool) {
	if prog == nil || prog.nativeHelper == nil {
		return nil, false
	}
	return prog.nativeHelper, true
}

func (RuntimeExecutionAdapter) InstallNativeHelper(prog *IRProgram, helper nativeF64Fn) {
	if prog != nil {
		prog.nativeHelper = helper
		prog.nativeChecked = true
	}
}

func (RuntimeExecutionAdapter) NativeHelperChecked(prog *IRProgram) bool {
	return prog != nil && prog.nativeChecked
}

func (RuntimeExecutionAdapter) CanTryMemNth(prog *IRProgram) bool {
	return prog != nil && !prog.memNthFailed
}

func (RuntimeExecutionAdapter) MarkMemNthFailed(prog *IRProgram) {
	if prog != nil {
		prog.memNthFailed = true
	}
}
