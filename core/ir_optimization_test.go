package core

import (
	"testing"
)

// --- guessFnParamFrame ---

func TestGuessFnParamFrameLetfnPixel(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, mandelbrotScript)
	loop := expr.(*LoopExpr)
	le := (*LetExpr)(loop)
	env := &LocalEnv{bindings: make([]Object, 0), frame: 0}
	for _, v := range le.values {
		env.bindings = append(env.bindings, Eval(v, env))
	}
	pixelFn := env.bindings[0].(*Fn)
	arity := pixelFn.fnExpr.arities[0]
	f := guessFnParamFrame(arity.body, len(arity.args))
	if f < 0 {
		t.Fatalf("guessFnParamFrame returned %d, want >= 0", f)
	}
	t.Logf("pixel param frame = %d", f)
}

func TestGuessFnParamFrameSimpleFn(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, `(let [f (fn [x y] (+ x y))] (f 1 2))`)
	le := expr.(*LetExpr)
	env := &LocalEnv{bindings: make([]Object, 0), frame: 0}
	fn := Eval(le.values[0], env).(*Fn)
	arity := fn.fnExpr.arities[0]
	f := guessFnParamFrame(arity.body, len(arity.args))
	if f < 0 {
		t.Fatalf("guessFnParamFrame returned %d for simple fn", f)
	}
}

// --- findLetFrame ---

func TestFindLetFrameSimpleLet(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, `(loop [i 0]
    (if (= i 3) 42
      (let [x (* i 2)]
        (recur (+ i x)))))`)
	loop := expr.(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Fatal("failed to compile simple let loop")
	}
}

func TestFindLetFrameNestedLet(t *testing.T) {
	clbgInit()
	r := Eval(compileBenchExpr(t, `(let [n 5]
    (loop [i 0 s 0]
      (if (= i n) s
        (let [x (* i i)]
          (recur (+ i 1) (+ s x))))))`), nil)
	if r == nil {
		t.Fatal("nested let loop returned nil")
	}
	if r.(Int).I != 30 {
		t.Fatalf("expected 30, got %s", r.ToString(false))
	}
}

// --- irCompileFn with captures ---

func TestIrCompileFnWithCaptures(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, mandelbrotScript)
	loop := expr.(*LoopExpr)
	le := (*LetExpr)(loop)
	env := &LocalEnv{bindings: make([]Object, 0), frame: 0}
	for _, v := range le.values {
		env.bindings = append(env.bindings, Eval(v, env))
	}
	pixelFn := env.bindings[0].(*Fn)
	prog := irCompileFn(pixelFn)
	if prog == nil {
		t.Fatal("irCompileFn failed for pixel fn")
	}
	// Verify correctness
	r := irExec(prog, []Object{Double{D: 0}, Double{D: 0}})
	if r == nil || r.(Int).I != 1 {
		t.Fatalf("pixel(0,0) = %v, want 1", r)
	}
	r2 := irExec(prog, []Object{Double{D: 2}, Double{D: 0}})
	if r2 == nil || r2.(Int).I != 0 {
		t.Fatalf("pixel(2,0) = %v, want 0", r2)
	}
}

func TestIrCompileFnFlip(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, fannkuchScript)
	le := expr.(*LetExpr)
	env := &LocalEnv{bindings: make([]Object, 0), frame: 0}
	for _, v := range le.values {
		env.bindings = append(env.bindings, Eval(v, env))
	}
	flipFn := env.bindings[2].(*Fn)
	prog := irCompileFn(flipFn)
	if prog == nil {
		t.Fatal("irCompileFn failed for flip fn")
	}
	perm := &ArrayVector{arr: []Object{Int{I: 1}, Int{I: 0}, Int{I: 2}}}
	r := irExec(prog, []Object{perm})
	if r == nil {
		t.Fatal("flip returned nil")
	}
	av := r.(*ArrayVector)
	if av.arr[0].(Int).I != 0 || av.arr[1].(Int).I != 1 {
		t.Fatalf("flip([1,0,2]) = %s, want [0 1 2]", r.ToString(false))
	}
}

// --- Frame stack (depth-limited) ---

func TestFrameStackBinaryTrees(t *testing.T) {
	clbgInit()
	r := Eval(compileBenchExpr(t, binaryTreesScript), nil)
	if r == nil {
		t.Fatal("binary-trees returned nil")
	}
	t.Logf("binary-trees result: %s", r.ToString(false))
}

func TestFrameStackRecursiveFib(t *testing.T) {
	clbgInit()
	r := Eval(compileBenchExpr(t, `(letfn [(fib [n] (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2)))))]
    (fib 10))`), nil)
	if r == nil || r.(Int).I != 55 {
		t.Fatalf("fib(10) = %v, want 55", r)
	}
}

func TestFrameStackDeepRecursion(t *testing.T) {
	clbgInit()
	// Ensure deep recursion > 256 (frame stack limit) still works
	r := Eval(compileBenchExpr(t, `(letfn [(countdown [n]
      (if (= n 0) 0 (+ 1 (countdown (- n 1)))))]
    (countdown 500))`), nil)
	if r == nil || r.(Int).I != 500 {
		t.Fatalf("countdown(500) = %v, want 500", r)
	}
}

// --- irValue 32-byte accessors ---

func TestIrValueBoolRoundtrip(t *testing.T) {
	for _, b := range []bool{true, false} {
		v := irMakeBool(b)
		if v.boolean() != b {
			t.Fatalf("irMakeBool(%v).boolean() = %v", b, v.boolean())
		}
	}
}

func TestIrValueCharRoundtrip(t *testing.T) {
	for _, r := range []rune{'A', '€', '日'} {
		v := irMakeChar(r)
		if v.char() != r {
			t.Fatalf("irMakeChar(%c).char() = %c", r, v.char())
		}
	}
}

func TestIrValueStringRoundtrip(t *testing.T) {
	v := irMakeString("hello", 5, true)
	if v.str() != "hello" || !v.isASCII() || v.i != 5 {
		t.Fatalf("string roundtrip failed: %q ascii=%v count=%d", v.str(), v.isASCII(), v.i)
	}
	v2 := irMakeString("日本語", 3, false)
	if v2.str() != "日本語" || v2.isASCII() || v2.i != 3 {
		t.Fatalf("unicode string roundtrip failed")
	}
}

func TestIrValueObjectRoundtrip(t *testing.T) {
	clbgInit()
	vec := &ArrayVector{arr: []Object{Int{I: 1}, Int{I: 2}}}
	v := irMakeObject(vec)
	got := v.obj()
	if got == nil {
		t.Fatal("irMakeObject roundtrip returned nil")
	}
	if got.(*ArrayVector).arr[0].(Int).I != 1 {
		t.Fatal("vector content mismatch")
	}
}

func TestIrValueIntVectorRoundtrip(t *testing.T) {
	iv := []int{1, 2, 3}
	v := irMakeIntVector(iv)
	got := v.intVec()
	if len(got) != 3 || got[2] != 3 {
		t.Fatalf("intVec roundtrip: %v", got)
	}
}

func TestIrValueStringIntMapRoundtrip(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	v := irMakeStringIntMap(m)
	got := v.stringIntMap()
	if got["a"] != 1 || got["b"] != 2 {
		t.Fatalf("stringIntMap roundtrip: %v", got)
	}
}

// --- noescape64 ---

func TestNoescape64Correctness(t *testing.T) {
	buf := [4]float64{1.0, 2.0, 3.0, 4.0}
	s := noescape64(buf[:3])
	if len(s) != 3 || s[0] != 1.0 || s[2] != 3.0 {
		t.Fatalf("noescape64 returned wrong values: %v", s)
	}
}

// --- Native helper ---

func TestNativeHelperSpectralA(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, spectralNormScript)
	le := expr.(*LetExpr)
	env := &LocalEnv{bindings: make([]Object, 0), frame: 0}
	env.bindings = append(env.bindings, Eval(le.values[0], env)) // n
	aFn := Eval(le.values[1], env).(*Fn)
	prog := irCompileFn(aFn)
	if prog == nil || prog.nativeHelper == nil {
		t.Fatal("A fn should have native helper")
	}
	// A(0,0) = 1/(0+1) = 1.0
	r := prog.nativeHelper([]float64{0, 0})
	if r != 1.0 {
		t.Fatalf("A(0,0) = %f, want 1.0", r)
	}
	// A(1,0) = 1/((1+0)(2)/2 + 2) = 1/3
	r2 := prog.nativeHelper([]float64{1, 0})
	if r2 < 0.333 || r2 > 0.334 {
		t.Fatalf("A(1,0) = %f, want ~0.333", r2)
	}
}

// --- Typed IR eligibility ---

func TestTypedIREligiblePureNumeric(t *testing.T) {
	clbgInit()
	// Pure float loop should be eligible
	expr := compileBenchExpr(t, `(loop [x 0.0 i 0]
    (if (= i 100) x (recur (+ x 1.5) (+ i 1))))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("failed to compile pure numeric loop")
	}
	a := AnalyzeIRProgram(prog)
	if !irTypedEligible(a) {
		t.Fatalf("pure numeric loop should be typed-eligible: %+v", a)
	}
}

func TestTypedIREligibleCallSlotWithNth(t *testing.T) {
	clbgInit()
	// Call-slot + nth pattern (spectral-norm j-loop)
	expr := compileBenchExpr(t, spectralNormScript)
	le := expr.(*LetExpr)
	env := &LocalEnv{bindings: make([]Object, 0), frame: 0}
	for _, v := range le.values[:3] {
		env.bindings = append(env.bindings, Eval(v, env))
	}
	mulAvFn := env.bindings[2].(*Fn)
	arity := mulAvFn.fnExpr.arities[0]
	bodyLoop := arity.body[0].(*LoopExpr)
	prog := irCompile(bodyLoop)
	if prog == nil {
		t.Fatal("failed to compile mul-Av loop")
	}
	a := AnalyzeIRProgram(prog)
	if !irTypedEligible(a) {
		t.Fatalf("call-slot+nth loop should be typed-eligible: %+v", a)
	}
}

// --- TransientVector in IR ---

func TestIRFirstTransientVector(t *testing.T) {
	clbgInit()
	r := Eval(compileBenchExpr(t, binaryTreesScript), nil)
	if r == nil {
		t.Fatal("binary-trees returned nil (irFirst/irNth TransientVector)")
	}
}

// --- irConj not map op ---

func TestIRConjNotMapOp(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, `(loop [i 0 v []]
    (if (= i 5) v (recur (+ i 1) (conj v i))))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("conj loop should compile")
	}
	a := AnalyzeIRProgram(prog)
	if a.HasMapOps {
		t.Fatal("conj should not set HasMapOps")
	}
}

// --- End-to-end benchmark correctness ---

func TestCLBGCorrectness(t *testing.T) {
	clbgInit()
	tests := []struct {
		name   string
		script string
	}{
		{"mandelbrot", mandelbrotScript},
		{"spectral-norm", spectralNormScript},
		{"binary-trees", binaryTreesScript},
		{"fannkuch", fannkuchScript},
		{"nbody", nbodyScript},
		{"fasta", fastaScript},
		{"pidigits", pidigitsScript},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Eval(compileBenchExpr(t, tt.script), nil)
			if r == nil {
				t.Fatalf("%s returned nil", tt.name)
			}
			t.Logf("%s = %s", tt.name, r.ToString(false))
		})
	}
}

func TestBuildNativeLoopWrapper(t *testing.T) {
	clbgInit()
	// spectral-norm's A fn has a native helper that can be wrapped
	expr := compileBenchExpr(t, spectralNormScript)
	le := expr.(*LetExpr)
	env := &LocalEnv{bindings: make([]Object, 0), frame: 0}
	env.bindings = append(env.bindings, Eval(le.values[0], env))
	aFn := Eval(le.values[1], env).(*Fn)
	fnProg := irGetFnProg(aFn)
	if fnProg == nil {
		t.Fatal("irGetFnProg returned nil for A fn")
	}
	if fnProg.nativeHelper == nil {
		t.Fatal("A fn should have nativeHelper")
	}
	t.Logf("A fn: nativeHelper present, slots=%d", fnProg.numSlots)
}

func TestIRFrameStackPushPop(t *testing.T) {
	fs := newIRFrameStack(4)
	slots := make([]Object, 4)
	slots[0] = Int{I: 42}
	slots[1] = Double{D: 3.14}

	fs.push(10, slots, 5)
	if fs.depth != 1 {
		t.Fatalf("depth = %d, want 1", fs.depth)
	}

	// Modify slots
	slots[0] = Int{I: 99}
	
	// Pop should restore original
	pc, sl := fs.pop(slots)
	if pc != 10 || sl != 5 {
		t.Fatalf("pop: pc=%d sl=%d, want 10, 5", pc, sl)
	}
	if slots[0].(Int).I != 42 {
		t.Fatalf("slot[0] = %v after pop, want 42", slots[0])
	}
}

// --- irCallSelf in typed executor ---

func TestTypedExecutorCallSelf(t *testing.T) {
	clbgInit()
	// Simple self-recursive fn
	r := Eval(compileBenchExpr(t, `(letfn [(sum [n]
      (if (= n 0) 0 (+ n (sum (- n 1)))))]
    (sum 10))`), nil)
	if r == nil || r.(Int).I != 55 {
		t.Fatalf("sum(10) = %v, want 55", r)
	}
}

func TestTypedExecutorBuildVec(t *testing.T) {
	clbgInit()
	r := Eval(compileBenchExpr(t, `(loop [i 0 v []]
    (if (= i 3) v (recur (+ i 1) (conj v [:item i]))))`), nil)
	if r == nil {
		t.Fatal("buildvec loop returned nil")
	}
	t.Logf("result: %s", r.ToString(false))
}

func TestTypedExecutorFirst(t *testing.T) {
	clbgInit()
	r := Eval(compileBenchExpr(t, `(let [v [:a :b :c]]
    (loop [i 0 result []]
      (if (= i 3) result
        (recur (+ i 1) (conj result (first v))))))`), nil)
	if r == nil {
		t.Fatal("first loop returned nil")
	}
}

// --- advance fn compilation ---

func TestAdvanceFnCompiles(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, nbodyScript)
	le := expr.(*LetExpr)
	env := &LocalEnv{bindings: make([]Object, 0), frame: 0}
	for _, v := range le.values {
		env.bindings = append(env.bindings, Eval(v, env))
	}
	advanceFn := env.bindings[14].(*Fn)
	prog := irCompileFn(advanceFn)
	if prog == nil {
		t.Fatal("advance fn should compile with depth limit 8")
	}
	t.Logf("advance: slots=%d caps=%d", prog.numSlots, len(prog.captureSlots))
}

func TestHeapPermFnCompiles(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, fannkuchScript)
	le := expr.(*LetExpr)
	env := &LocalEnv{bindings: make([]Object, 0), frame: 0}
	for _, v := range le.values {
		env.bindings = append(env.bindings, Eval(v, env))
	}
	letfnLoop := le.body[0].(*LoopExpr)
	letfnLe := (*LetExpr)(letfnLoop)
	env2 := &LocalEnv{bindings: make([]Object, 0), frame: 0, parent: env}
	for _, v := range letfnLe.values {
		env2.bindings = append(env2.bindings, Eval(v, env2))
	}
	heapPermFn := env2.bindings[0].(*Fn)
	prog := irCompileFn(heapPermFn)
	if prog == nil {
		t.Fatal("heap-perm fn should compile with depth limit 8")
	}
	t.Logf("heap-perm: slots=%d caps=%d hasSelf=%v", prog.numSlots, len(prog.captureSlots), prog.hasSelf)
}

// --- Transient ops in typed executor ---

func TestTypedExecutorTransientOps(t *testing.T) {
	clbgInit()
	// This triggers irToTransient + irAssocBang + irToPersistent via escape analysis
	r := Eval(compileBenchExpr(t, `(loop [i 0 v [0 0 0 0 0]]
    (if (= i 5) v (recur (+ i 1) (assoc v i (* i i)))))`), nil)
	if r == nil {
		t.Fatal("transient loop returned nil")
	}
	av := r.(*ArrayVector)
	if av.arr[4].(Int).I != 16 {
		t.Fatalf("v[4] = %v, want 16", av.arr[4])
	}
}

func TestTypedExecutorNthOnObject(t *testing.T) {
	clbgInit()
	r := Eval(compileBenchExpr(t, `(let [v [10 20 30]]
    (loop [i 0 s 0]
      (if (= i 3) s (recur (+ i 1) (+ s (nth v i))))))`), nil)
	if r == nil || r.(Int).I != 60 {
		t.Fatalf("nth sum = %v, want 60", r)
	}
}

func TestTypedExecutorStringOps(t *testing.T) {
	clbgInit()
	r := Eval(compileBenchExpr(t, `(loop [i 0 s ""]
    (if (= i 3) s (recur (+ i 1) (str s (str i)))))`), nil)
	if r == nil {
		t.Fatal("str loop returned nil")
	}
	if r.(String).S != "012" {
		t.Fatalf("str result = %q, want \"012\"", r.(String).S)
	}
}

func TestIrValueKeywordRoundtrip(t *testing.T) {
	clbgInit()
	kw := Eval(compileBenchExpr(t, `:test`), nil).(Keyword)
	v := objectToIRValue(kw)
	if v.tag != irValKeyword {
		t.Fatalf("expected irValKeyword, got %d", v.tag)
	}
	back := v.object().(Keyword)
	if *back.name != *kw.name {
		t.Fatalf("keyword roundtrip: %v != %v", *back.name, *kw.name)
	}
}

func TestIrValueKeywordEquality(t *testing.T) {
	clbgInit()
	kw1 := objectToIRValue(Eval(compileBenchExpr(t, `:leaf`), nil))
	kw2 := objectToIRValue(Eval(compileBenchExpr(t, `:leaf`), nil))
	kw3 := objectToIRValue(Eval(compileBenchExpr(t, `:node`), nil))
	
	eq12, ok := irValueEq(kw1, kw2)
	if !ok || !eq12.boolean() {
		t.Fatal(":leaf should equal :leaf")
	}
	eq13, ok := irValueEq(kw1, kw3)
	if !ok || eq13.boolean() {
		t.Fatal(":leaf should not equal :node")
	}
}

// --- irIntCast and irSubs opcodes ---

func TestIRIntCast(t *testing.T) {
	clbgInit()
	tests := []struct{ expr, want string }{
		{`(let [f (fn [c] (int c))] (f \A))`, "65"},
		{`(let [f (fn [c] (int c))] (f \0))`, "48"},
		{`(let [f (fn [n] (int n))] (f 3.7))`, "3"},
		{`(let [f (fn [n] (int n))] (f 42))`, "42"},
	}
	for _, tt := range tests {
		r := Eval(compileBenchExpr(t, tt.expr), nil)
		if r.ToString(false) != tt.want {
			t.Errorf("%s = %s, want %s", tt.expr, r.ToString(false), tt.want)
		}
	}
}

func TestIRSubs(t *testing.T) {
	clbgInit()
	tests := []struct{ expr, want string }{
		{`(let [f (fn [s] (subs s 1 3))] (f "hello"))`, "el"},
		{`(let [f (fn [s] (subs s 0 5))] (f "hello"))`, "hello"},
		{`(let [f (fn [s i] (subs s i))] (f "hello" 2))`, "llo"},
	}
	for _, tt := range tests {
		r := Eval(compileBenchExpr(t, tt.expr), nil)
		if r.ToString(false) != tt.want {
			t.Errorf("%s = %s, want %s", tt.expr, r.ToString(false), tt.want)
		}
	}
}
