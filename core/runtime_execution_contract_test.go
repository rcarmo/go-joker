package core

import "testing"

func TestRuntimeExecutionAdapterPrepareCallSlotsInstallsCaptures(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	prog := &IRProgram{
		numSlots:        3,
		captureKeys:     []bindingKey{{index: 0}},
		captureSlotIdxs: []int{2},
	}
	env := &LocalEnv{bindings: []Object{MakeString("captured")}}
	args := []Object{MakeInt(1)}
	full := adapter.PrepareCallSlots(prog, args, env)
	if len(full) != 3 || full[0] != args[0] || full[2].(String).S != "captured" {
		t.Fatalf("prepared call slots mismatch: %#v", full)
	}
	if got := adapter.PrepareCallSlots(&IRProgram{}, args, env); len(got) != 1 || got[0] != args[0] {
		t.Fatalf("capture-free call should reuse args: %#v", got)
	}
}

func TestRuntimeExecutionAdapterInstallsTypedEnvCaptures(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	objects := adapter.ObjectsFromTypedValues([]irValue{objectToIRValue(MakeInt(1)), objectToIRValue(MakeString("x"))}, make([]Object, 2))
	if len(objects) != 2 || !objects[0].Equals(MakeInt(1)) || !objects[1].Equals(MakeString("x")) {
		t.Fatalf("ObjectsFromTypedValues = %#v", objects)
	}
	prog := &IRProgram{
		numSlots:        2,
		captureKeys:     []bindingKey{{index: 0}},
		captureSlotIdxs: []int{1},
	}
	env := &LocalEnv{bindings: []Object{MakeInt(42)}}
	slots := make([]irValue, 2)
	adapter.InstallTypedEnvCaptures(prog, slots, env)
	if slots[1].tag != irValInt || slots[1].i != 42 {
		t.Fatalf("typed env capture slot = %#v", slots[1])
	}
}

func TestRuntimeExecutionAdapterProgramMetadata(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	fnExpr := &FnExpr{}
	prog := &IRProgram{
		numSlots:        3,
		code:            []byte{1, 2, 3},
		constants:       []Object{MakeInt(7)},
		fnExprs:         []*FnExpr{fnExpr},
		captureSlotIdxs: []int{2},
		captureSlots:    []Object{MakeString("captured")},
	}
	if got := adapter.ProgramNumSlots(prog); got != 3 {
		t.Fatalf("ProgramNumSlots = %d, want 3", got)
	}
	if got := adapter.ProgramCode(prog); len(got) != 3 || got[0] != 1 {
		t.Fatalf("ProgramCode = %#v", got)
	}
	if got, ok := adapter.ProgramConstant(prog, 0); !ok || !got.Equals(MakeInt(7)) {
		t.Fatalf("ProgramConstant = %#v, %v", got, ok)
	}
	if got := adapter.ProgramConstants(prog); len(got) != 1 || !got[0].Equals(MakeInt(7)) {
		t.Fatalf("ProgramConstants = %#v", got)
	}
	if got, ok := adapter.ProgramFnExpr(prog, 0); !ok || got != fnExpr {
		t.Fatalf("ProgramFnExpr = %#v, %v", got, ok)
	}
	subProg := &IRProgram{numSlots: 1}
	prog.arityPrograms = map[int]*IRProgram{1: subProg}
	if got := adapter.DispatchArityProgram(prog, 1); got != subProg {
		t.Fatalf("DispatchArityProgram exact = %#v", got)
	}
	if got := adapter.DispatchArityProgram(prog, 2); got != nil {
		t.Fatalf("DispatchArityProgram miss = %#v", got)
	}
	prog.variadicProg = subProg
	prog.variadicMinArgs = 2
	if got := adapter.DispatchArityProgram(prog, 3); got != subProg {
		t.Fatalf("DispatchArityProgram variadic = %#v", got)
	}
	variadicOnly := &IRProgram{numSlots: 2, variadicMinArgs: 2}
	if got := adapter.DispatchArityProgram(variadicOnly, 1); got != nil {
		t.Fatalf("DispatchArityProgram variadic-only under-arity = %#v", got)
	}
	if got := adapter.DispatchArityProgram(variadicOnly, 2); got != variadicOnly {
		t.Fatalf("DispatchArityProgram variadic-only exact/min = %#v", got)
	}
	fnObj := &Fn{irProg: prog, env: &LocalEnv{bindings: []Object{MakeInt(1)}}}
	if got, ok := adapter.FnProgram(fnObj); !ok || got != prog {
		t.Fatalf("FnProgram = %#v, %v", got, ok)
	}
	failedFn := &Fn{irProg: irCompileFailed}
	if got, ok := adapter.FnProgram(failedFn); ok || got != nil {
		t.Fatalf("FnProgram should hide compile-failed sentinel, got %#v, %v", got, ok)
	}
	if slots, ok := adapter.FnCallSlots(fnObj, prog, []Object{MakeInt(2)}); !ok || len(slots) == 0 || !slots[0].Equals(MakeInt(2)) {
		t.Fatalf("FnCallSlots = %#v, %v", slots, ok)
	}
	if !adapter.ProgramHasCaptureSlots(prog) {
		t.Fatal("ProgramHasCaptureSlots returned false")
	}
	objectSlots := []Object{NIL, NIL, NIL}
	if !adapter.ApplyProgramCaptureSlots(prog, objectSlots) || !objectSlots[2].Equals(MakeString("captured")) {
		t.Fatalf("ApplyProgramCaptureSlots = %#v", objectSlots)
	}
	typedSlots := make([]irValue, 3)
	if !adapter.ApplyProgramTypedCaptureSlots(prog, typedSlots) || !typedSlots[2].object().Equals(MakeString("captured")) {
		t.Fatalf("ApplyProgramTypedCaptureSlots = %#v", typedSlots)
	}
	typedSlots[1] = objectToIRValue(MakeInt(99))
	if !adapter.ClearTypedNonCaptureSlots(prog, typedSlots, 1) || !typedSlots[2].object().Equals(MakeString("captured")) || typedSlots[1] != (irValue{}) {
		t.Fatalf("ClearTypedNonCaptureSlots = %#v", typedSlots)
	}
	prog.captureSlotSet = []bool{false}
	if adapter.ClearTypedNonCaptureSlots(prog, typedSlots, 1) {
		t.Fatal("ClearTypedNonCaptureSlots accepted short capture slot set")
	}
	prog.captureSlotSet = nil
	idxs, captures := adapter.ProgramCaptureSlots(prog)
	if len(idxs) != 1 || idxs[0] != 2 || len(captures) != 1 || !captures[0].Equals(MakeString("captured")) {
		t.Fatalf("ProgramCaptureSlots = %#v, %#v", idxs, captures)
	}
	if info := adapter.ProgramEscapeInfo(prog); info == nil || len(info.SafeMutableSlots) != 3 {
		t.Fatalf("ProgramEscapeInfo = %#v", info)
	}
	if analysis := adapter.ProgramAnalysis(prog); analysis.NumOps == 0 {
		t.Fatalf("ProgramAnalysis.NumOps = %d, want non-zero", analysis.NumOps)
	}
	if adapter.ProgramNumSlots(nil) != 0 || adapter.ProgramCode(nil) != nil {
		t.Fatal("nil program metadata should be empty")
	}
	if _, ok := adapter.ProgramConstant(prog, 1); ok {
		t.Fatal("ProgramConstant accepted out-of-range index")
	}
	if _, ok := adapter.ProgramFnExpr(prog, 1); ok {
		t.Fatal("ProgramFnExpr accepted out-of-range index")
	}
}

func TestRuntimeExecutionAdapterExecutionFailureFlags(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	prog := &IRProgram{}
	if !adapter.CanExecuteIR(prog) || !adapter.CanExecuteTypedIR(prog) {
		t.Fatal("fresh program should be executable by boxed and typed IR")
	}
	adapter.MarkTypedExecutionFailed(prog)
	if !prog.typedFailed || !adapter.CanExecuteIR(prog) || adapter.CanExecuteTypedIR(prog) {
		t.Fatalf("typed failure should only disable typed IR: %#v", prog)
	}
	adapter.MarkBoxedExecutionFailed(prog)
	if !prog.execFailed || adapter.CanExecuteIR(prog) || adapter.CanExecuteTypedIR(prog) {
		t.Fatalf("boxed failure should disable all IR execution: %#v", prog)
	}
	if adapter.CanExecuteIR(nil) || adapter.CanExecuteTypedIR(nil) {
		t.Fatal("nil program must not be executable")
	}
}

func TestRuntimeExecutionAdapterMakeFnCapturesSlots(t *testing.T) {
	expr := compileBenchExpr(t, `(fn [y] y)`)
	fnExpr := expr.(*FnExpr)
	adapter := RuntimeExecutionAdapter{}
	fnObj := adapter.MakeFn(fnExpr, []Object{MakeInt(10)})
	fn, ok := fnObj.(*Fn)
	if !ok {
		t.Fatalf("MakeFn returned %T, want *Fn", fnObj)
	}
	if fn.env == nil || len(fn.env.bindings) != 1 || !fn.env.bindings[0].Equals(MakeInt(10)) {
		t.Fatalf("MakeFn did not capture slots: %#v", fn.env)
	}
}

func TestRuntimeExecutionAdapterCallObject(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	args, ok := adapter.CallArgs(NewArrayVectorFrom(MakeInt(1), MakeInt(2)))
	if !ok || len(args) != 2 || !args[1].Equals(MakeInt(2)) {
		t.Fatalf("CallArgs = %#v, %v", args, ok)
	}
	if _, ok := adapter.CallArgs(MakeInt(1)); ok {
		t.Fatal("CallArgs accepted non-seqable")
	}
	fn := Proc{Name: "contract-call", Fn: func(args []Object) Object { return MakeInt(len(args)) }}
	got, ok := adapter.CallObject(fn, []Object{MakeInt(1), MakeInt(2)})
	if !ok || !got.Equals(MakeInt(2)) {
		t.Fatalf("CallObject = %#v, %v", got, ok)
	}
	if _, ok := adapter.CallObject(MakeInt(1), nil); ok {
		t.Fatal("CallObject accepted non-callable")
	}
	got, ok = adapter.CallObjectWithSyntheticCallExpr(fn, []Object{MakeInt(1)})
	if !ok || !got.Equals(MakeInt(1)) {
		t.Fatalf("CallObjectWithSyntheticCallExpr = %#v, %v", got, ok)
	}
	grt := currentGRT()
	prevExpr := grt.currentExpr
	panicking := Proc{Name: "contract-panic", Fn: func(args []Object) Object { panic("boom") }}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("CallObjectWithSyntheticCallExpr did not propagate panic")
			}
		}()
		_, _ = adapter.CallObjectWithSyntheticCallExpr(panicking, nil)
	}()
	if grt.currentExpr != prevExpr {
		t.Fatal("CallObjectWithSyntheticCallExpr did not restore current expression after panic")
	}
}

func TestRuntimeExecutionAdapterCollectionOps(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	vec := NewArrayVectorFrom(MakeInt(1), MakeInt(2))
	if got, ok := adapter.Nth(vec, 1); !ok || !got.Equals(MakeInt(2)) {
		t.Fatalf("Nth = %#v, %v", got, ok)
	}
	if _, ok := adapter.Nth(vec, 9); ok {
		t.Fatal("Nth accepted out-of-range index")
	}
	mapObj := EmptyArrayMap().Assoc(MakeString("k"), MakeInt(7)).(Object)
	if got := adapter.Get(mapObj, MakeString("k"), NIL); !got.Equals(MakeInt(7)) {
		t.Fatalf("Get = %#v", got)
	}
	if got := adapter.Get(vec, MakeInt(9), MakeInt(42)); !got.Equals(MakeInt(42)) {
		t.Fatalf("Get default = %#v", got)
	}
	if got, ok := adapter.Assoc(vec, MakeInt(0), MakeInt(9)); !ok {
		t.Fatalf("Assoc returned %#v, %v", got, ok)
	} else if ok, val := got.(Gettable).Get(MakeInt(0)); !ok || !val.Equals(MakeInt(9)) {
		t.Fatalf("Assoc value = %#v, %v", val, ok)
	}
	if got, ok := adapter.Conj(vec, MakeInt(3)); !ok || got.(Counted).Count() != 3 {
		t.Fatalf("Conj returned %#v, %v", got, ok)
	}
	if got, ok := adapter.First(vec); !ok || !got.Equals(MakeInt(1)) {
		t.Fatalf("First returned %#v, %v", got, ok)
	}
	if got, ok := adapter.First(EmptyArrayVector()); !ok || got != NIL {
		t.Fatalf("First empty returned %#v, %v", got, ok)
	}
	if got := adapter.BuildVector([]Object{MakeInt(4), MakeInt(5)}); got.(Counted).Count() != 2 {
		t.Fatalf("BuildVector returned %#v", got)
	}
	transient := ToTransient(vec)
	if !adapter.HasMutableSlotCandidate([]Object{MakeInt(1), vec}) {
		t.Fatal("HasMutableSlotCandidate missed vector")
	}
	if adapter.HasMutableSlotCandidate([]Object{MakeInt(1), MakeSymbol("x")}) {
		t.Fatal("HasMutableSlotCandidate should ignore non-candidate objects")
	}
	if got := adapter.MutableSlotObject(vec, &EscapeInfo{SafeMutableSlots: []bool{true}}, 0); got == vec {
		t.Fatalf("MutableSlotObject did not convert vector: %#v", got)
	}
	if got := adapter.PersistentResult(transient); got.(Counted).Count() != 2 {
		t.Fatalf("PersistentResult = %#v", got)
	}
	transientObj, ok := adapter.ToTransient(vec)
	if !ok {
		t.Fatal("ToTransient rejected vector")
	}
	if got, ok := adapter.AssocBang(transientObj, MakeInt(1), MakeInt(8)); !ok {
		t.Fatalf("AssocBang returned %#v, %v", got, ok)
	} else if got, ok := adapter.ToPersistent(got); !ok || got.(Indexed).Nth(1) != MakeInt(8) {
		t.Fatalf("ToPersistent after AssocBang = %#v, %v", got, ok)
	}
}

func TestRuntimeExecutionAdapterStringOps(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	if got := adapter.Str1(MakeChar('x')); !got.Equals(MakeString("x")) {
		t.Fatalf("Str1 char = %#v", got)
	}
	if got := adapter.Str2(MakeString("a"), MakeChar('b')); !got.Equals(MakeString("ab")) {
		t.Fatalf("Str2 = %#v", got)
	}
	if got, ok := adapter.Count(MakeString("abc")); !ok || got != 3 {
		t.Fatalf("Count = %d, %v", got, ok)
	}
	prog := &IRProgram{constants: []Object{MakeString("abc")}}
	if got, ok := adapter.NthASCIIStringConst(prog, 0, 1); !ok || !got.Equals(MakeChar('b')) {
		t.Fatalf("NthASCIIStringConst = %#v, %v", got, ok)
	}
	cur := NewStringCursor("x")
	if got, ok := adapter.CursorChar(cur); !ok || !got.Equals(MakeChar('x')) {
		t.Fatalf("CursorChar = %#v, %v", got, ok)
	}
	if got, ok := adapter.CursorDone(cur); !ok || got.(Boolean).B {
		t.Fatalf("CursorDone = %#v, %v", got, ok)
	}
	if got, ok := adapter.CursorNext(cur); !ok || got == cur {
		t.Fatalf("CursorNext = %#v, %v", got, ok)
	}
}

func TestRuntimeExecutionAdapterErrorf(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	err := adapter.Errorf("contract %d", 42)
	msg, ok := err.Message().(String)
	if err == nil || !ok || msg.S != "contract 42" {
		t.Fatalf("Errorf = %#v", err)
	}
}

func TestRuntimeExecutionAdapterApplyCaptureSlots(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	slots := []Object{NIL, NIL, NIL}
	if !adapter.ApplyCaptureSlots(slots, []int{2, 0}, []Object{MakeInt(20), MakeInt(10)}) {
		t.Fatal("ApplyCaptureSlots returned false for valid captures")
	}
	if !slots[0].Equals(MakeInt(10)) || !slots[2].Equals(MakeInt(20)) {
		t.Fatalf("capture slots = %#v", slots)
	}
	if adapter.ApplyCaptureSlots(slots, []int{3}, []Object{MakeInt(1)}) {
		t.Fatal("ApplyCaptureSlots accepted out-of-range slot")
	}
	if adapter.ApplyCaptureSlots(slots, []int{1, 2}, []Object{MakeInt(1)}) {
		t.Fatal("ApplyCaptureSlots accepted mismatched metadata")
	}
}

func TestRuntimeExecutionAdapterApplyTypedCaptureSlots(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	slots := make([]irValue, 2)
	if !adapter.ApplyTypedCaptureSlots(slots, []int{1}, []Object{MakeInt(42)}) {
		t.Fatal("ApplyTypedCaptureSlots returned false for valid captures")
	}
	if slots[1].tag != irValInt || slots[1].i != 42 {
		t.Fatalf("typed capture slot = %#v", slots[1])
	}
	if adapter.ApplyTypedCaptureSlots(slots, []int{-1}, []Object{MakeInt(1)}) {
		t.Fatal("ApplyTypedCaptureSlots accepted out-of-range slot")
	}
}

func TestRuntimeExecutionAdapterThrow(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	defer func() {
		r := recover()
		err, ok := r.(Error)
		if !ok {
			t.Fatalf("Throw panic = %T, want core Error", r)
		}
		msg := err.Message().(String)
		if msg.S != "boom" {
			t.Fatalf("Throw message = %q, want boom", msg.S)
		}
	}()
	adapter.Throw(MakeString("boom"))
}

func TestRuntimeExecutionAdapterNativeHelperState(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	prog := &IRProgram{}
	if adapter.HasNativeHelper(prog) || adapter.NativeHelperChecked(prog) {
		t.Fatal("fresh program should not have a checked native helper")
	}
	if helper, ok := adapter.NativeHelper(prog); helper != nil || ok {
		t.Fatalf("NativeHelper = %v, %v; want nil, false", helper, ok)
	}
	helper := nativeF64Fn(func(args []float64) float64 { return args[0] + 1 })
	adapter.InstallNativeHelper(prog, helper)
	got, ok := adapter.NativeHelper(prog)
	if !ok || got([]float64{2}) != 3 {
		t.Fatal("native helper was not installed through adapter")
	}
	if !adapter.HasNativeHelper(prog) || !adapter.NativeHelperChecked(prog) {
		t.Fatal("adapter did not expose installed native helper state")
	}
}

func TestRuntimeExecutionAdapterMemNthFallbackState(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	prog := &IRProgram{}
	if !adapter.CanTryMemNth(prog) {
		t.Fatal("fresh program should allow mem-nth attempt")
	}
	adapter.MarkMemNthFailed(prog)
	if adapter.CanTryMemNth(prog) || !prog.memNthFailed {
		t.Fatal("MarkMemNthFailed did not disable mem-nth attempts")
	}
	if adapter.CanTryMemNth(nil) {
		t.Fatal("nil program must not allow mem-nth attempts")
	}
}
