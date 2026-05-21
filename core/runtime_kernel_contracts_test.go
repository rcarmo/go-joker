package core

import (
	"io"
	"io/fs"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corert "github.com/rcarmo/go-joker/core/runtime"

	coregenerated "github.com/rcarmo/go-joker/core/generated"
	coreirx "github.com/rcarmo/go-joker/core/ir"
	corereader "github.com/rcarmo/go-joker/core/reader"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
	"github.com/rcarmo/go-joker/tests/clbgscripts"
)

// ---- concurrency_ext_test.go ----
func requireKeyword(tb testing.TB, obj coretypes.Object, want string) {
	tb.Helper()
	got, ok := obj.(coretypes.Keyword)
	if !ok {
		tb.Fatalf("expected Keyword(%s), got %T (%s)", want, obj, obj.ToString(false))
	}
	if got.ToString(false) != want {
		tb.Fatalf("expected Keyword(%s), got %s", want, got.ToString(false))
	}
}

func TestChannelCloseIsIdempotentUnderConcurrency(t *testing.T) {
	ch := corert.NewObjectChannel(make(chan corert.FutureResult, 1))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch.Close()
		}()
	}
	wg.Wait()
	if !ch.IsClosed() {
		t.Fatal("channel should report closed after concurrent Close calls")
	}
	if ch.Send(coretypes.MakeInt(1)) {
		t.Fatal("Send on closed channel should return false")
	}
}

func TestCoreAsyncNamespaceAliases(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [c (clojure.core.async/chan 1)]
    (clojure.core.async/>!! c 42)
    (clojure.core.async/<!! c))`), 42)
}

func TestCoreAsyncGoLoopAndPipeline(t *testing.T) {
	requireString(t, evalTestScript(t, `(let [c (clojure.core.async/chan 1)]
    (clojure.core.async/go-loop [i 0]
      (if (< i 3)
        (do (clojure.core.async/>! c i) (recur (inc i)))
        (clojure.core.async/close! c)))
    (str (clojure.core.async/<!! c) ":" (clojure.core.async/<!! c) ":" (clojure.core.async/<!! c) ":" (clojure.core.async/<!! c)))`), "0:1:2:")
}

func TestCoreAsyncMapFilterMergeSplit(t *testing.T) {
	requireString(t, evalTestScript(t, `(let [mapped (clojure.core.async/map< inc (clojure.core.async/to-chan [1 2 3]))
        filtered (clojure.core.async/filter< even? mapped)]
    (str (clojure.core.async/<!! filtered) ":" (clojure.core.async/<!! filtered)))`), "2:4")

	requireString(t, evalTestScript(t, `(let [m (clojure.core.async/merge [(clojure.core.async/to-chan [1]) (clojure.core.async/to-chan [2])])
        xs [(clojure.core.async/<!! m) (clojure.core.async/<!! m)]]
    (str (count (set xs)) ":" (contains? (set xs) 1) ":" (contains? (set xs) 2)))`), "2:true:true")

	requireString(t, evalTestScript(t, `(let [[evens odds] (clojure.core.async/split even? (clojure.core.async/to-chan [1 2]))]
    (str (clojure.core.async/<!! odds) ":" (clojure.core.async/<!! evens)))`), "1:2")
}

func TestCoreAsyncMultAndPub(t *testing.T) {
	requireString(t, evalTestScript(t, `(let [src (clojure.core.async/chan 1)
        m (clojure.core.async/mult src)
        t1 (clojure.core.async/chan 1)
        t2 (clojure.core.async/chan 1)]
    (clojure.core.async/tap m t1)
    (clojure.core.async/tap m t2)
    (clojure.core.async/>!! src :x)
    (str (clojure.core.async/<!! t1) ":" (clojure.core.async/<!! t2)))`), ":x::x")

	requireString(t, evalTestScript(t, `(let [src (clojure.core.async/chan 1)
        p (clojure.core.async/pub src identity)
        out (clojure.core.async/chan 1)]
    (clojure.core.async/sub p :topic out)
    (clojure.core.async/>!! src :topic)
    (str (clojure.core.async/<!! out)))`), ":topic")
}

func TestCoreAsyncReduceIntoAndCallbacks(t *testing.T) {
	requireInt(t, evalTestScript(t, `(clojure.core.async/<!! (clojure.core.async/reduce + 0 (clojure.core.async/to-chan [1 2 3])))`), 6)

	requireString(t, evalTestScript(t, `(str (clojure.core.async/<!! (clojure.core.async/into [] (clojure.core.async/to-chan [1 2]))))`), "[1 2]")

	requireString(t, evalTestScript(t, `(let [c (clojure.core.async/chan 1)
        p (promise)]
    (clojure.core.async/take! c #(deliver p %))
    (clojure.core.async/put! c 9)
    (str @p))`), "9")

	requireString(t, evalTestScript(t, `(let [c (clojure.core.async/chan 1)
        p (promise)]
    (clojure.core.async/take! c #(deliver p %))
    (clojure.core.async/close! c)
    (str @p))`), "")
}

func TestConcurrencyTimeoutRejectsTooLarge(t *testing.T) {
	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		_ = checkedMillisecondDuration(int(^uint(0)>>1), "timeout")
	}()
	if !didPanic {
		t.Fatal("timeout accepted overflowing millisecond value")
	}
}

func TestConcurrencyTimeoutRejectsNegative(t *testing.T) {
	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		_ = evalTestScript(t, `(timeout -1)`)
	}()
	if !didPanic {
		t.Fatal("timeout accepted negative milliseconds")
	}
}

func TestConcurrencyTimeoutAndAltsDefault(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [ch (chan)]
  (first (alts! [ch] :default 42)))`), 42)

	requireKeyword(t, evalTestScript(t, `(let [ch (chan)]
  (second (alts! [ch] :default 42)))`), ":default")
}

func TestConcurrencyAltsRejectsOddOptions(t *testing.T) {
	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		_ = evalTestScript(t, `(let [ch (chan)] (alts! [ch] :default))`)
	}()
	if !didPanic {
		t.Fatal("alts! accepted odd option list")
	}
}

func TestConcurrencyAltsClosedPutReturnsFalse(t *testing.T) {
	requireBool(t, evalTestScript(t, `(let [c (chan)]
  (close! c)
  (first (alts! [[c 1]])))`), false)
}

func TestConcurrencyFuturePromiseAgent(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [f (future (+ 40 2))] @f)`), 42)
	requireInt(t, evalTestScript(t, `(let [p (promise)] (deliver p 7) @p)`), 7)
	requireInt(t, evalTestScript(t, `(let [a (agent 0)]
  (send a + 10)
  (send a + 20)
  (send a + 12)
  (await a)
  @a)`), 42)
}

func TestConcurrencyPmapAndPcalls(t *testing.T) {
	requireInt(t, evalTestScript(t, `(reduce + 0 (pmap inc [1 2 3 4]))`), 14)
	requireInt(t, evalTestScript(t, `(reduce + 0 (pcalls #(+ 1 1) #(+ 2 2) #(+ 3 3)))`), 12)
}

func TestConcurrencyPcallsRecursiveFn(t *testing.T) {
	requireInt(t, evalTestScript(t, `(letfn [(fib [n]
  (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2)))))]
  (reduce + 0 (pcalls (fn [] (fib 20))
                      (fn [] (fib 20))
                      (fn [] (fib 20)))))`), 20295)
}

func TestConcurrencyPcallsPanicPropagates(t *testing.T) {
	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		_ = evalTestScript(t, `(pcalls (fn [] 1) (fn [] (/ 1 0)))`)
	}()
	if !didPanic {
		t.Fatalf("expected panic to propagate from pcalls worker")
	}
}

// ---- construction_boundary_guard_test.go ----
func TestCollectionConstructorsBuildExpectedValues(t *testing.T) {
	items := []coretypes.Object{coretypes.MakeInt(1), coretypes.MakeString("two")}

	vector := corecollections.NewVectorFrom(items...)
	if vector.Count() != 2 || !vector.At(1).Equals(items[1]) {
		t.Fatalf("VectorFrom mismatch: %s", vector.ToString(false))
	}
	fromSeq := corecollections.NewVectorFromSeq(corecollections.NewListFrom(items...).Seq())
	if !fromSeq.Equals(vector) {
		t.Fatalf("VectorFromSeq = %s, want %s", fromSeq.ToString(false), vector.ToString(false))
	}
	array := corecollections.NewArrayVectorFrom(items...)
	if array.Count() != 2 || !array.At(0).Equals(items[0]) {
		t.Fatalf("ArrayVectorFrom mismatch: %s", array.ToString(false))
	}
	if corecollections.EmptyVector().Count() != 0 || corecollections.EmptyArrayVector().Count() != 0 {
		t.Fatal("empty vector constructors should return empty collections")
	}

	key := coretypes.MakeKeyword(STRINGS.Intern, "k")
	value := coretypes.MakeInt(42)
	arrayMap := corecollections.EmptyArrayMap().Assoc(key, value).(coretypes.Map)
	hashMap := corecollections.NewHashMap(key, value)
	if !arrayMap.Equals(hashMap) || arrayMap.Hash() != hashMap.Hash() {
		t.Fatalf("map constructors should build equivalent maps: %s / %s", arrayMap.ToString(false), hashMap.ToString(false))
	}
	set := corecollections.EmptySet().Conj(coretypes.MakeInt(1)).Conj(coretypes.MakeInt(2)).(*corecollections.MapSet)
	fromSetSeq := corecollections.NewSetFromSeq(corecollections.NewListFrom(coretypes.MakeInt(2), coretypes.MakeInt(1)).Seq())
	if !set.Equals(fromSetSeq) || set.Hash() != fromSetSeq.Hash() {
		t.Fatalf("set constructors should build equivalent sets: %s / %s", set.ToString(false), fromSetSeq.ToString(false))
	}
}

func TestReaderConstructionCallSitesUseAdapter(t *testing.T) {
	direct := regexp.MustCompile(`(^|[^.])\b(NewReader|TryRead|Read|NewLiteralExpr|NewSurrogateExpr)\(|&(VectorExpr|MapExpr|SetExpr)\b`)
	allowed := map[string]bool{
		"parse.go":                            true, // legacy/parser expression constructor implementations when split.
		"read.go":                             true, // legacy reader read-loop implementations when split.
		"runtime_kernel.go":                   true, // currently coalesces parser/reader/evaluator construction.
		"reader.go":                           true, // owns Reader constructor implementation.
		"reader_construction.go":              true,
		"construction_boundary_guard_test.go": true,
	}
	assertNoDirectConstructionOutside(t, direct, allowed)
}

func assertNoDirectConstructionOutside(t *testing.T, direct *regexp.Regexp, allowed map[string]bool) {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		base := filepath.Base(file)
		if strings.HasSuffix(base, "_test.go") && base != "construction_boundary_guard_test.go" {
			continue
		}
		if allowed[base] {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for lineNo, line := range strings.Split(string(data), "\n") {
			if direct.MatchString(line) {
				t.Fatalf("%s:%d uses direct construction instead of adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
		}
	}
}

// ---- escape_analysis_contract_test.go ----
func TestEscapeAnalysisMarksCallArgumentsUnsafe(t *testing.T) {
	prog := (&IRProgram{
		code: []byte{
			irLoadSlot, 0, 0,
			irCallSlot, 0, 1, 0, 1,
			irReturn,
		},
		numSlots: 2,
	}).refreshModel()
	info := analyzeEscapes(prog)
	if len(info.SafeMutableSlots) != 2 {
		t.Fatalf("SafeMutableSlots len = %d, want 2", len(info.SafeMutableSlots))
	}
	if info.SafeMutableSlots[0] {
		t.Fatal("slot passed as call argument should be unsafe for transient mutation")
	}
	if !info.SafeMutableSlots[1] {
		t.Fatal("unused call function slot should remain safe")
	}
}

func TestEscapeAnalysisTracksStringBuilderSlots(t *testing.T) {
	prog := (&IRProgram{
		code: []byte{
			irLoadSlot, 0, 0,
			irLoadSlot, 0, 1,
			irStr2,
			irReturn,
		},
		numSlots: 2,
	}).refreshModel()
	info := analyzeEscapes(prog)
	if !info.StringBuilderSlots[0] {
		t.Fatal("left str operand slot should be marked append-builder candidate")
	}
	if !info.StringPrependSlots[1] {
		t.Fatal("right str operand slot should be marked prepend-builder candidate")
	}
}

// ---- loop_optimization_test.go ----
func TestIRCompilesLiteralMapInitializer(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 m {}]
  (if (= i 4)
    (get m :a 0)
    (recur (inc i) (assoc m :a (inc (get m :a 0))))))`)
	d := explainFirstLoop(expr)
	if !d.Compiled {
		t.Fatalf("expected IR compile for empty map initializer, got %q", d.Reason)
	}
	requireInt(t, Eval(expr, nil), 4)
}

func TestIRDiagnosticsUnsupportedDynamicMapLiteral(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0]
  (if (= i 1)
    {:x i}
    (recur (inc i))))`)
	d := explainFirstLoop(expr)
	if d.Compiled {
		t.Fatalf("expected dynamic map literal rejection")
	}
	if !strings.Contains(d.Reason, "dynamic map literal") {
		t.Fatalf("expected dynamic map literal reason, got %q", d.Reason)
	}
}

func TestIRDiagnosticsSpecificUnsupportedCallable(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0]
  (if (= i 1)
    i
    ((fn [x] x) (inc i))))`)
	d := explainFirstLoop(expr)
	if d.Compiled {
		t.Fatalf("expected unsupported callable rejection")
	}
	if !strings.Contains(d.Reason, "unsupported callable expression") {
		t.Fatalf("expected unsupported callable reason, got %q", d.Reason)
	}
}

// --- guessFnParamFrame ---

func TestGuessFnParamFrameLetfnPixel(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, clbgscripts.MandelbrotScript)
	loop := expr.(*LoopExpr)
	le := (*LetExpr)(loop)
	env := &LocalEnv{bindings: make([]coretypes.Object, 0), frame: 0}
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
	env := &LocalEnv{bindings: make([]coretypes.Object, 0), frame: 0}
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
	if r.(coretypes.Int).I != 30 {
		t.Fatalf("expected 30, got %s", r.ToString(false))
	}
}

// --- irCompileFn with captures ---

func TestIrCompileFnWithCaptures(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, clbgscripts.MandelbrotScript)
	loop := expr.(*LoopExpr)
	le := (*LetExpr)(loop)
	env := &LocalEnv{bindings: make([]coretypes.Object, 0), frame: 0}
	for _, v := range le.values {
		env.bindings = append(env.bindings, Eval(v, env))
	}
	pixelFn := env.bindings[0].(*Fn)
	prog := irCompileFn(pixelFn)
	if prog == nil {
		t.Fatal("irCompileFn failed for pixel fn")
	}
	// Verify correctness
	r := irExec(prog, []coretypes.Object{coretypes.Double{D: 0}, coretypes.Double{D: 0}})
	if r == nil || r.(coretypes.Number).Int().I != 1 {
		t.Fatalf("pixel(0,0) = %v, want 1", r)
	}
	r2 := irExec(prog, []coretypes.Object{coretypes.Double{D: 2}, coretypes.Double{D: 0}})
	if r2 == nil || r2.(coretypes.Number).Int().I != 0 {
		t.Fatalf("pixel(2,0) = %v, want 0", r2)
	}
}

func TestIrCompileFnFlip(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, clbgscripts.FannkuchScript)
	le := expr.(*LetExpr)
	env := &LocalEnv{bindings: make([]coretypes.Object, 0), frame: 0}
	for _, v := range le.values {
		env.bindings = append(env.bindings, Eval(v, env))
	}
	flipFn := env.bindings[2].(*Fn)
	prog := irCompileFn(flipFn)
	if prog == nil {
		t.Fatal("irCompileFn failed for flip fn")
	}
	perm := &corecollections.ArrayVector{Arr: []coretypes.Object{coretypes.Int{I: 1}, coretypes.Int{I: 0}, coretypes.Int{I: 2}}}
	r := irExec(prog, []coretypes.Object{perm})
	if r == nil {
		t.Fatal("flip returned nil")
	}
	av := r.(*corecollections.ArrayVector)
	if av.Arr[0].(coretypes.Int).I != 0 || av.Arr[1].(coretypes.Int).I != 1 {
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
	if r == nil || r.(coretypes.Int).I != 55 {
		t.Fatalf("fib(10) = %v, want 55", r)
	}
}

func TestFrameStackDeepRecursion(t *testing.T) {
	clbgInit()
	// Ensure deep recursion > 256 (frame stack limit) still works
	r := Eval(compileBenchExpr(t, `(letfn [(countdown [n]
      (if (= n 0) 0 (+ 1 (countdown (- n 1)))))]
    (countdown 500))`), nil)
	if r == nil || r.(coretypes.Int).I != 500 {
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
	vec := &corecollections.ArrayVector{Arr: []coretypes.Object{coretypes.Int{I: 1}, coretypes.Int{I: 2}}}
	v := irMakeObject(vec)
	got := v.obj()
	if got == nil {
		t.Fatal("irMakeObject roundtrip returned nil")
	}
	if got.(*corecollections.ArrayVector).Arr[0].(coretypes.Int).I != 1 {
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
	s := coreirx.Float64(buf[:3])
	if len(s) != 3 || s[0] != 1.0 || s[2] != 3.0 {
		t.Fatalf("noescape64 returned wrong values: %v", s)
	}
}

// --- coretypes.Native helper ---

func TestNativeHelperSpectralA(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, spectralNormScript)
	le := expr.(*LetExpr)
	env := &LocalEnv{bindings: make([]coretypes.Object, 0), frame: 0}
	env.bindings = append(env.bindings, Eval(le.values[0], env)) // n
	aFn := Eval(le.values[1], env).(*Fn)
	prog := irCompileFn(aFn)
	nativeHelper, ok := runtimeExec.NativeHelper(prog)
	if prog == nil || !ok {
		t.Fatal("A fn should have native helper")
	}
	// A(0,0) = 1/(0+1) = 1.0
	r := nativeHelper([]float64{0, 0})
	if r != 1.0 {
		t.Fatalf("A(0,0) = %f, want 1.0", r)
	}
	// A(1,0) = 1/((1+0)(2)/2 + 2) = 1/3
	r2 := nativeHelper([]float64{1, 0})
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
	env := &LocalEnv{bindings: make([]coretypes.Object, 0), frame: 0}
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

// --- coretypes.TransientVector in IR ---

func TestIRFirstTransientVector(t *testing.T) {
	clbgInit()
	r := Eval(compileBenchExpr(t, binaryTreesScript), nil)
	if r == nil {
		t.Fatal("binary-trees returned nil (irFirst/irNth coretypes.TransientVector)")
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
		{"mandelbrot", clbgscripts.MandelbrotScript},
		{"spectral-norm", spectralNormScript},
		{"binary-trees", binaryTreesScript},
		{"fannkuch", clbgscripts.FannkuchScript},
		{"nbody", nbodyScript},
		{"fasta", clbgscripts.FastaScript},
		{"pidigits", clbgscripts.PidigitsScript},
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
	env := &LocalEnv{bindings: make([]coretypes.Object, 0), frame: 0}
	env.bindings = append(env.bindings, Eval(le.values[0], env))
	aFn := Eval(le.values[1], env).(*Fn)
	fnProg := irGetFnProg(aFn)
	if fnProg == nil {
		t.Fatal("irGetFnProg returned nil for A fn")
	}
	if !runtimeExec.HasNativeHelper(fnProg) {
		t.Fatal("A fn should have nativeHelper")
	}
	t.Logf("A fn: nativeHelper present, slots=%d", fnProg.numSlots)
}

func TestIRFrameStackPushPop(t *testing.T) {
	fs := coreirx.NewFrameStack[coretypes.Object](4)
	defer coreirx.ReleaseFrameStack(fs)
	slots := make([]coretypes.Object, 4)
	slots[0] = coretypes.Int{I: 42}
	slots[1] = coretypes.Double{D: 3.14}

	fs.Push(10, slots, 5)

	// Modify slots
	slots[0] = coretypes.Int{I: 99}

	// Pop should restore original
	pc, sl := fs.Pop(slots)
	if pc != 10 || sl != 5 {
		t.Fatalf("pop: pc=%d sl=%d, want 10, 5", pc, sl)
	}
	if slots[0].(coretypes.Int).I != 42 {
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
	if r == nil || r.(coretypes.Int).I != 55 {
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
	env := &LocalEnv{bindings: make([]coretypes.Object, 0), frame: 0}
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

func TestFannkuchHelperFnCompiles(t *testing.T) {
	clbgInit()
	expr := compileBenchExpr(t, clbgscripts.FannkuchScript)
	le := expr.(*LetExpr)
	env := &LocalEnv{bindings: make([]coretypes.Object, 0), frame: 0}
	for _, v := range le.values {
		env.bindings = append(env.bindings, Eval(v, env))
	}
	countFlipsFn := env.bindings[3].(*Fn)
	prog := irCompileFn(countFlipsFn)
	if prog == nil {
		t.Fatal("fannkuch count-flips fn should compile")
	}
	t.Logf("count-flips: slots=%d caps=%d hasSelf=%v", prog.numSlots, len(prog.captureSlots), prog.hasSelf)
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
	av := r.(*corecollections.ArrayVector)
	if av.Arr[4].(coretypes.Int).I != 16 {
		t.Fatalf("v[4] = %v, want 16", av.Arr[4])
	}
}

func TestTypedExecutorNthOnObject(t *testing.T) {
	clbgInit()
	r := Eval(compileBenchExpr(t, `(let [v [10 20 30]]
    (loop [i 0 s 0]
      (if (= i 3) s (recur (+ i 1) (+ s (nth v i))))))`), nil)
	if r == nil || r.(coretypes.Int).I != 60 {
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
	if r.(coretypes.String).S != "012" {
		t.Fatalf("str result = %q, want \"012\"", r.(coretypes.String).S)
	}
}

func TestIrValueKeywordRoundtrip(t *testing.T) {
	clbgInit()
	kw := Eval(compileBenchExpr(t, `:test`), nil).(coretypes.Keyword)
	v := objectToIRValue(kw)
	if v.tag != irValKeyword {
		t.Fatalf("expected irValKeyword, got %d", v.tag)
	}
	back := v.object().(coretypes.Keyword)
	if back.Name() != kw.Name() {
		t.Fatalf("keyword roundtrip: %v != %v", back.Name(), kw.Name())
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

func TestIRGuessLoopFramePrefersRecurBindings(t *testing.T) {
	expr := compileTestExpr(t, `(let [limit 4]
  (loop [i 0 acc 0]
    (if (< i limit)
      (let [x (loop [j 0 s 0]
                (if (= j i) s (recur (inc j) (+ s j))))]
        (recur (inc i) (+ acc x)))
      acc)))`)
	outer := expr.(*LetExpr).body[0].(*LoopExpr)
	if prog := irCompile(outer); prog == nil {
		t.Fatal("expected outer loop with nested loop/captures to compile")
	}
	requireInt(t, Eval(expr, nil), 4)
}

// ---- loop_wasm_diagnostics_test.go ----
func TestIRDiagnosticsPureWASM(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 acc 0]
  (if (= i 100)
    acc
    (recur (inc i) (+ acc i))))`)
	d := explainFirstLoop(expr)
	if !d.Compiled {
		t.Fatalf("expected IR compile, got reason %q", d.Reason)
	}
	if !d.WASM.Eligible {
		t.Fatalf("expected pure WASM eligibility, got %+v", d.WASM)
	}
	if !d.UsesWASM {
		t.Fatalf("expected UsesWASM for pure numeric loop: %+v", d)
	}
}

func TestIRDiagnosticsCollectionNeedsImports(t *testing.T) {
	expr := compileTestExpr(t, `(let [ks [:a :b]]
  (loop [i 0 m {}]
    (if (= i 10)
      (get m :a 0)
      (let [k (nth ks (rem i 2))]
        (recur (inc i) (assoc m k (+ 1 (get m k 0))))))))`)
	d := explainFirstLoop(expr)
	if !d.Compiled {
		t.Fatalf("expected IR compile, got reason %q", d.Reason)
	}
	if d.WASM.Eligible {
		t.Fatalf("expected WASM rejection for collection loop")
	}
	if !d.WASM.HasImports || !strings.Contains(d.WASM.Reason, "host imports") {
		t.Fatalf("expected host-import diagnostic, got %+v", d.WASM)
	}
}

func TestIRDiagnosticsStringOpNotWASM(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 s ""]
  (if (= i 3)
    s
    (recur (inc i) (str s "x"))))`)
	d := explainFirstLoop(expr)
	if !d.Compiled {
		t.Fatalf("expected IR compile, got reason %q", d.Reason)
	}
	if d.WASM.Eligible {
		t.Fatalf("expected WASM rejection for string op")
	}
	if d.WASM.OpName != "irStr2" {
		t.Fatalf("expected irStr2 diagnostic, got %+v", d.WASM)
	}
}

func TestIRDiagnosticsNoLoop(t *testing.T) {
	d := explainFirstLoop(compileTestExpr(t, `(+ 1 2)`))
	if d.Compiled || d.Reason != "no loop expression found" {
		t.Fatalf("unexpected diagnostic: %+v", d)
	}
}

// ---- numeric_promotion_test.go ----
func requirePanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s did not panic", name)
		}
	}()
	fn()
}

func TestBitOpsRejectInvalidIndexesAndShifts(t *testing.T) {
	requirePanic(t, "negative bit index", func() { procBitSet([]coretypes.Object{coretypes.MakeInt(0), coretypes.MakeInt(-1)}) })
	requirePanic(t, "too-large bit index", func() { procBitTest([]coretypes.Object{coretypes.MakeInt(0), coretypes.MakeInt(strconv.IntSize)}) })
	requirePanic(t, "negative shift count", func() { procBitShiftLeft([]coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(-1)}) })
}

func TestIntArithmeticPromotesToBigIntOnOverflow(t *testing.T) {
	if got := procAdd([]coretypes.Object{coretypes.Int{I: coretypes.MaxInt}, coretypes.Int{I: 1}}); got.GetType() != TYPE.BigInt || got.ToString(false) != "9223372036854775808N" {
		t.Fatalf("add promotion mismatch: %T %s", got, got.ToString(false))
	}
	if got := procSubtract([]coretypes.Object{coretypes.Int{I: coretypes.MinInt}, coretypes.Int{I: 1}}); got.GetType() != TYPE.BigInt || got.ToString(false) != "-9223372036854775809N" {
		t.Fatalf("subtract promotion mismatch: %T %s", got, got.ToString(false))
	}
	if got := procMultiply([]coretypes.Object{coretypes.Int{I: coretypes.MaxInt}, coretypes.Int{I: 2}}); got.GetType() != TYPE.BigInt || got.ToString(false) != "18446744073709551614N" {
		t.Fatalf("multiply promotion mismatch: %T %s", got, got.ToString(false))
	}
}

func TestIncDecPromoteToBigIntOnOverflow(t *testing.T) {
	if got := procInc([]coretypes.Object{coretypes.Int{I: coretypes.MaxInt}}); got.GetType() != TYPE.BigInt {
		t.Fatalf("inc did not promote: %T %s", got, got.ToString(false))
	}
	if got := procDec([]coretypes.Object{coretypes.Int{I: coretypes.MinInt}}); got.GetType() != TYPE.BigInt {
		t.Fatalf("dec did not promote: %T %s", got, got.ToString(false))
	}
}

func TestBigDecimalArithmeticKeepsBigFloat(t *testing.T) {
	a, _ := coretypes.MakeBigFloatWithOrig("0.1", "0.1M")
	b, _ := coretypes.MakeBigFloatWithOrig("0.2", "0.2M")
	got := procAdd([]coretypes.Object{a, b})
	if got.GetType() != TYPE.BigFloat || !strings.HasPrefix(got.ToString(false), "0.3") || !strings.HasSuffix(got.ToString(false), "M") {
		t.Fatalf("big decimal add mismatch: %T %s", got, got.ToString(false))
	}
}

func TestBigIntIntPanicsOutsideNativeRange(t *testing.T) {
	tooLarge := coretypes.MakeBigInt(new(big.Int).Add(coretypes.MaxIntBig, big.NewInt(1)))
	defer func() {
		if recover() == nil {
			t.Fatal("coretypes.BigInt.Int should panic outside native int range")
		}
	}()
	_ = tooLarge.Int()
}

func TestBigIntIntConvertsWithinNativeRange(t *testing.T) {
	got := coretypes.MakeBigInt(big.NewInt(42)).Int()
	if got.I != 42 {
		t.Fatalf("coretypes.BigInt.Int = %d, want 42", got.I)
	}
}

func TestBigIntDoubleUsesFullMagnitude(t *testing.T) {
	large := coretypes.MakeBigInt(new(big.Int).Lsh(big.NewInt(1), 70))
	got := large.Double().D
	want := math.Pow(2, 70)
	if got != want {
		t.Fatalf("coretypes.BigInt.Double = %.0f, want %.0f", got, want)
	}
}

type contractFileInfo struct {
	size int64
}

func (fi contractFileInfo) Name() string       { return "contract" }
func (fi contractFileInfo) Size() int64        { return fi.size }
func (fi contractFileInfo) Mode() fs.FileMode  { return 0o644 }
func (fi contractFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (fi contractFileInfo) IsDir() bool        { return false }
func (fi contractFileInfo) Sys() any           { return nil }

func TestFileInfoMapPromotesLargeSize(t *testing.T) {
	m := corert.FileInfoMap("contract", contractFileInfo{size: math.MaxInt64}, STRINGS.Intern)
	found, got := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "size"))
	if !found {
		t.Fatal("FileInfoMap missing :size")
	}
	if math.MaxInt64 > int64(coretypes.MaxInt) {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("file size type = %s, want coretypes.BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("file size type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestRatioOrIntUsesNativeIntRange(t *testing.T) {
	tooLargeFor32Bit := new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 40))
	got := coretypes.RatioOrInt(tooLargeFor32Bit)
	if strconv.IntSize == 32 {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("32-bit ratio integer promotion type = %s, want coretypes.BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("64-bit ratio integer promotion type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestRatioOrIntWithOriginalPreservesBigIntOriginal(t *testing.T) {
	tooLarge := new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 70))
	got := coretypes.RatioOrIntWithOriginal("1180591620717411303424/1", tooLarge)
	bi, ok := got.(*coretypes.BigInt)
	if !ok {
		t.Fatalf("large ratio integer type = %T, want *coretypes.BigInt", got)
	}
	if bi.Original != "1180591620717411303424/1" {
		t.Fatalf("coretypes.BigInt original = %q", bi.Original)
	}
}

// ---- object_protocol_contract_test.go ----
func TestExtendTypeInternalRejectsOddMethodPairs(t *testing.T) {
	proto := &Protocol{name: coretypes.MakeSymbol(STRINGS.Intern, "AuditProto")}
	extendType := GLOBAL_ENV.CoreNamespace.Resolve("__extend-type")
	if extendType == nil {
		t.Fatal("missing __extend-type var")
	}
	proc, ok := extendType.Value.(Proc)
	if !ok {
		t.Fatalf("__extend-type value = %T, want Proc", extendType.Value)
	}
	assertPanics(t, "odd __extend-type method pairs", func() {
		proc.Call([]coretypes.Object{proto, coretypes.MakeString("AuditType"), coretypes.MakeString("method")})
	})
}

func TestCountedIndexedVectorContract(t *testing.T) {
	items := []coretypes.Object{coretypes.MakeInt(1), coretypes.MakeString("two"), coretypes.MakeKeyword(STRINGS.Intern, "three")}
	vectors := []struct {
		name string
		v    coretypes.Object
	}{
		{name: "array", v: corecollections.NewArrayVectorFrom(items...)},
		{name: "vector", v: corecollections.NewVectorFrom(items...)},
		{name: "persistent", v: corecollections.PersistentVectorFrom(items)},
	}
	for _, tc := range vectors {
		ci, ok := tc.v.(coretypes.CountedIndexed)
		if !ok {
			t.Fatalf("%s does not implement coretypes.CountedIndexed", tc.name)
		}
		if ci.Count() != len(items) {
			t.Fatalf("%s Count = %d", tc.name, ci.Count())
		}
		for i, want := range items {
			if !ci.At(i).Equals(want) {
				t.Fatalf("%s At(%d) = %s, want %s", tc.name, i, ci.At(i).ToString(false), want.ToString(false))
			}
		}
		if got := tc.v.ToString(false); got != "[1 two :three]" {
			t.Fatalf("%s ToString = %q", tc.name, got)
		}
	}
	for i := range vectors {
		for j := range vectors {
			if !vectors[i].v.Equals(vectors[j].v) {
				t.Fatalf("%s should equal %s", vectors[i].name, vectors[j].name)
			}
			if vectors[i].v.Hash() != vectors[j].v.Hash() {
				t.Fatalf("%s hash %d != %s hash %d", vectors[i].name, vectors[i].v.Hash(), vectors[j].name, vectors[j].v.Hash())
			}
		}
	}
}

func TestAssociativeMapContract(t *testing.T) {
	entries := []coretypes.Object{coretypes.MakeKeyword(STRINGS.Intern, "a"), coretypes.MakeInt(1), coretypes.MakeKeyword(STRINGS.Intern, "b"), coretypes.MakeString("two")}
	maps := []struct {
		name string
		m    coretypes.Map
	}{
		{name: "array", m: corecollections.EmptyArrayMap().Assoc(entries[0], entries[1]).Assoc(entries[2], entries[3]).(coretypes.Map)},
		{name: "hash", m: corecollections.NewHashMap(entries...)},
	}
	for _, tc := range maps {
		if tc.m.Count() != 2 {
			t.Fatalf("%s Count = %d", tc.name, tc.m.Count())
		}
		if found, got := tc.m.Get(coretypes.MakeKeyword(STRINGS.Intern, "a")); !found || !got.Equals(coretypes.MakeInt(1)) {
			t.Fatalf("%s Get(:a) = %v %v", tc.name, found, got)
		}
		updated := tc.m.Assoc(coretypes.MakeKeyword(STRINGS.Intern, "a"), coretypes.MakeInt(10)).(coretypes.Map)
		if found, got := updated.Get(coretypes.MakeKeyword(STRINGS.Intern, "a")); !found || !got.Equals(coretypes.MakeInt(10)) {
			t.Fatalf("%s updated Get(:a) = %v %v", tc.name, found, got)
		}
		if found, got := tc.m.Get(coretypes.MakeKeyword(STRINGS.Intern, "a")); !found || !got.Equals(coretypes.MakeInt(1)) {
			t.Fatalf("%s Assoc mutated original: %v %v", tc.name, found, got)
		}
	}
	if !maps[0].m.Equals(maps[1].m) || !maps[1].m.Equals(maps[0].m) {
		t.Fatal("array map and hash map should compare equal")
	}
	if maps[0].m.Hash() != maps[1].m.Hash() {
		t.Fatalf("map hash mismatch: array=%d hash=%d", maps[0].m.Hash(), maps[1].m.Hash())
	}
}

func TestMapSetZeroValueContract(t *testing.T) {
	var set corecollections.MapSet
	if set.Count() != 0 || !set.Seq().IsEmpty() {
		t.Fatalf("zero-value set should be empty")
	}
	if ok, got := set.Get(coretypes.MakeInt(1)); ok || got != nil {
		t.Fatalf("zero-value set get = %v %v", ok, got)
	}
	if !set.Add(coretypes.MakeInt(1)) || set.Count() != 1 {
		t.Fatalf("zero-value set add failed")
	}
}

func TestSetContract(t *testing.T) {
	set := corecollections.EmptySet().Conj(coretypes.MakeInt(1)).Conj(coretypes.MakeInt(2)).(*corecollections.MapSet)
	if set.Count() != 2 {
		t.Fatalf("Count = %d, want 2", set.Count())
	}
	if found, got := set.Get(coretypes.MakeInt(1)); !found || !got.Equals(coretypes.MakeInt(1)) {
		t.Fatalf("Get(1) = %v %v", found, got)
	}
	if got := set.Call([]coretypes.Object{coretypes.MakeInt(2)}); !got.Equals(coretypes.MakeInt(2)) {
		t.Fatalf("Call(2) = %s", got.ToString(false))
	}
	if got := set.Call([]coretypes.Object{coretypes.MakeInt(3)}); got != NIL {
		t.Fatalf("Call(3) = %s, want nil", got.ToString(false))
	}
	removed := set.Disjoin(coretypes.MakeInt(1)).(*corecollections.MapSet)
	if found, _ := removed.Get(coretypes.MakeInt(1)); found {
		t.Fatal("Disjoin result still contains removed value")
	}
	if found, _ := set.Get(coretypes.MakeInt(1)); !found {
		t.Fatal("Disjoin mutated original set")
	}
	set2 := corecollections.NewSetFromSeq(corecollections.NewListFrom(coretypes.MakeInt(2), coretypes.MakeInt(1)).Seq())
	if !set.Equals(set2) || set.Hash() != set2.Hash() {
		t.Fatalf("equivalent sets should compare equal with same hash: %s / %s", set.ToString(false), set2.ToString(false))
	}
	meta := corecollections.EmptyArrayMap().Assoc(coretypes.MakeKeyword(STRINGS.Intern, "tag"), coretypes.MakeString("kept")).(coretypes.Map)
	withMeta := set.WithMeta(meta).(*corecollections.MapSet)
	conjMeta := withMeta.Conj(coretypes.MakeInt(3)).(coretypes.Meta)
	if found, got := conjMeta.GetMeta().Get(coretypes.MakeKeyword(STRINGS.Intern, "tag")); !found || !got.Equals(coretypes.MakeString("kept")) {
		t.Fatal("set Conj did not preserve metadata")
	}
	disjoinMeta := withMeta.Disjoin(coretypes.MakeInt(1)).(coretypes.Meta)
	if found, got := disjoinMeta.GetMeta().Get(coretypes.MakeKeyword(STRINGS.Intern, "tag")); !found || !got.Equals(coretypes.MakeString("kept")) {
		t.Fatal("set Disjoin did not preserve metadata")
	}
}

func TestSortedCollectionContract(t *testing.T) {
	sortedMapProc := GLOBAL_ENV.CoreNamespace.Resolve("sorted-map").Value.(Proc)
	sortedSetProc := GLOBAL_ENV.CoreNamespace.Resolve("sorted-set").Value.(Proc)
	sortedQProc := GLOBAL_ENV.CoreNamespace.Resolve("sorted?").Value.(Proc)
	subseqProc := GLOBAL_ENV.CoreNamespace.Resolve("subseq").Value.(Proc)
	rsubseqProc := GLOBAL_ENV.CoreNamespace.Resolve("rsubseq").Value.(Proc)
	sortedMapByProc := GLOBAL_ENV.CoreNamespace.Resolve("sorted-map-by").Value.(Proc)
	sortedSetByProc := GLOBAL_ENV.CoreNamespace.Resolve("sorted-set-by").Value.(Proc)
	desc := Proc{Name: "desc", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		return coretypes.MakeBoolean(compareObjects(args[0], args[1]) > 0)
	}}

	m := sortedMapProc.Call([]coretypes.Object{coretypes.MakeInt(2), coretypes.MakeString("b"), coretypes.MakeInt(1), coretypes.MakeString("a")}).(coretypes.Map)
	if got := sortedQProc.Call([]coretypes.Object{m}); !got.Equals(coretypes.Boolean{B: true}) {
		t.Fatalf("sorted? sorted-map = %s", got.ToString(false))
	}
	entries := sortedEntries(m)
	if len(entries) != 2 || !rangeKey(entries[0]).Equals(coretypes.MakeInt(1)) || !rangeKey(entries[1]).Equals(coretypes.MakeInt(2)) {
		t.Fatalf("sorted-map entries not ordered: %#v", entries)
	}
	if found, got := m.Get(coretypes.MakeInt(1)); !found || !got.Equals(coretypes.MakeString("a")) {
		t.Fatalf("sorted-map lookup = %v %v", found, got)
	}
	dup := sortedMapProc.Call([]coretypes.Object{coretypes.MakeInt(1), coretypes.MakeString("old"), coretypes.MakeInt(1), coretypes.MakeString("new")}).(coretypes.Map)
	if dup.Count() != 1 {
		t.Fatalf("sorted-map duplicate count = %d, want 1", dup.Count())
	}
	if found, got := dup.Get(coretypes.MakeInt(1)); !found || !got.Equals(coretypes.MakeString("new")) {
		t.Fatalf("sorted-map duplicate lookup = %v %v", found, got)
	}

	s := sortedSetProc.Call([]coretypes.Object{coretypes.MakeInt(3), coretypes.MakeInt(1), coretypes.MakeInt(2)}).(*corecollections.MapSet)
	if got := sortedQProc.Call([]coretypes.Object{s}); !got.Equals(coretypes.Boolean{B: true}) {
		t.Fatalf("sorted? sorted-set = %s", got.ToString(false))
	}
	setEntries := sortedEntries(s)
	if len(setEntries) != 3 || !setEntries[0].Equals(coretypes.MakeInt(1)) || !setEntries[2].Equals(coretypes.MakeInt(3)) {
		t.Fatalf("sorted-set entries not ordered: %#v", setEntries)
	}

	sub := subseqProc.Call([]coretypes.Object{s, Proc{Fn: procGte, Name: "procGte"}, coretypes.MakeInt(2)}).(coretypes.Seq)
	if corecollections.SeqCount(sub) != 2 || !sub.First().Equals(coretypes.MakeInt(2)) || !corecollections.Second(sub).Equals(coretypes.MakeInt(3)) {
		t.Fatalf("subseq contract failed: %s", sub.ToString(false))
	}
	rsub := rsubseqProc.Call([]coretypes.Object{s, Proc{Fn: procGte, Name: "procGte"}, coretypes.MakeInt(2)}).(coretypes.Seq)
	if corecollections.SeqCount(rsub) != 2 || !rsub.First().Equals(coretypes.MakeInt(3)) || !corecollections.Second(rsub).Equals(coretypes.MakeInt(2)) {
		t.Fatalf("rsubseq contract failed: %s", rsub.ToString(false))
	}
	mBy := sortedMapByProc.Call([]coretypes.Object{desc, coretypes.MakeInt(1), coretypes.MakeString("a"), coretypes.MakeInt(3), coretypes.MakeString("c"), coretypes.MakeInt(2), coretypes.MakeString("b")}).(coretypes.Map)
	mByEntries := sortedEntries(mBy)
	if len(mByEntries) != 3 || !rangeKey(mByEntries[0]).Equals(coretypes.MakeInt(3)) || !rangeKey(mByEntries[2]).Equals(coretypes.MakeInt(1)) {
		t.Fatalf("sorted-map-by should preserve comparator order: %v", mByEntries)
	}
	byParity := Proc{Name: "parity", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		ai := args[0].(coretypes.Int).I % 2
		bi := args[1].(coretypes.Int).I % 2
		if ai < bi {
			return coretypes.Int{I: -1}
		}
		if ai > bi {
			return coretypes.Int{I: 1}
		}
		return coretypes.Int{I: 0}
	}}
	mByDup := sortedMapByProc.Call([]coretypes.Object{byParity, coretypes.MakeInt(1), coretypes.MakeString("one"), coretypes.MakeInt(3), coretypes.MakeString("three"), coretypes.MakeInt(2), coretypes.MakeString("two")}).(coretypes.Map)
	mByDupEntries := sortedEntries(mByDup)
	if len(mByDupEntries) != 2 || !rangeKey(mByDupEntries[1]).Equals(coretypes.MakeInt(3)) {
		t.Fatalf("sorted-map-by comparator duplicate entries = %v", mByDupEntries)
	}
	if found, got := mByDup.Get(coretypes.MakeInt(3)); !found || !got.Equals(coretypes.MakeString("three")) {
		t.Fatalf("sorted-map-by comparator duplicate lookup = %v %v", found, got)
	}
	sBy := sortedSetByProc.Call([]coretypes.Object{desc, coretypes.MakeInt(1), coretypes.MakeInt(3), coretypes.MakeInt(2)}).(*corecollections.MapSet)
	sByEntries := sortedEntries(sBy)
	if len(sByEntries) != 3 || !sByEntries[0].Equals(coretypes.MakeInt(3)) || !sByEntries[2].Equals(coretypes.MakeInt(1)) {
		t.Fatalf("sorted-set-by should preserve comparator order: %v", sByEntries)
	}
}

func TestTransientContract(t *testing.T) {
	vec := corecollections.NewArrayVectorFrom(coretypes.MakeInt(1), coretypes.MakeInt(2))
	tv := coretypes.ToTransient(vec.Arr)
	if _, ok := any(tv).(coretypes.CountedIndexed); !ok {
		t.Fatal("coretypes.TransientVector should implement coretypes.CountedIndexed")
	}
	if tv.Count() != 2 || !tv.At(1).Equals(coretypes.MakeInt(2)) {
		t.Fatalf("transient vector Count/At mismatch: count=%d at1=%s", tv.Count(), tv.At(1).ToString(false))
	}
	tv.AssocInPlace(coretypes.MakeInt(1), coretypes.MakeInt(20)).ConjInPlace(coretypes.MakeInt(3))
	if tv.Count() != 3 || !tv.At(1).Equals(coretypes.MakeInt(20)) || !tv.At(2).Equals(coretypes.MakeInt(3)) {
		t.Fatalf("transient vector mutation mismatch: %s %s %s", tv.At(0).ToString(false), tv.At(1).ToString(false), tv.At(2).ToString(false))
	}
	pv := coretypes.EnsureObjectIsCountedIndexed(tv.ToPersistent(), "")
	if pv.Count() != 3 || !pv.At(1).Equals(coretypes.MakeInt(20)) {
		t.Fatalf("persistent vector round trip mismatch")
	}
	assertPanics(t, "mutating frozen transient vector", func() { tv.ConjInPlace(coretypes.MakeInt(4)) })

	m := corecollections.EmptyArrayMap().Assoc(coretypes.MakeKeyword(STRINGS.Intern, "a"), coretypes.MakeInt(1)).Assoc(coretypes.MakeString("s"), coretypes.MakeInt(2)).(coretypes.Map)
	tm := coretypes.MapToTransient(m)
	tm.AssocInPlace(coretypes.MakeKeyword(STRINGS.Intern, "a"), coretypes.MakeInt(10)).AssocInPlace(coretypes.MakeString("t"), coretypes.MakeInt(3))
	if tm.Count() != 3 {
		t.Fatalf("transient map Count = %d, want 3", tm.Count())
	}
	if found, got := tm.Get(coretypes.MakeKeyword(STRINGS.Intern, "a")); !found || !got.Equals(coretypes.MakeInt(10)) {
		t.Fatalf("transient map keyword get = %v %v", found, got)
	}
	if found, got := tm.Get(coretypes.MakeString("t")); !found || !got.Equals(coretypes.MakeInt(3)) {
		t.Fatalf("transient map string get = %v %v", found, got)
	}
	pm := tm.ToPersistent().(coretypes.Map)
	if pm.Count() != 3 {
		t.Fatalf("persistent map Count = %d, want 3", pm.Count())
	}
	if found, got := pm.Get(coretypes.MakeString("t")); !found || !got.Equals(coretypes.MakeInt(3)) {
		t.Fatalf("persistent map string get = %v %v", found, got)
	}
	assertPanics(t, "mutating frozen transient map", func() { tm.AssocInPlace(coretypes.MakeKeyword(STRINGS.Intern, "z"), coretypes.MakeInt(0)) })
	if got := procIsTransient([]coretypes.Object{coretypes.ToTransient(corecollections.NewArrayVectorFrom().Arr)}); !got.Equals(coretypes.Boolean{B: true}) {
		t.Fatal("transient? should recognize transient vectors")
	}
	if got := procIsTransient([]coretypes.Object{coretypes.MapToTransient(nil)}); !got.Equals(coretypes.Boolean{B: true}) {
		t.Fatal("transient? should recognize transient maps")
	}
	if got := procIsTransient([]coretypes.Object{corecollections.NewArrayVectorFrom()}); !got.Equals(coretypes.Boolean{B: false}) {
		t.Fatal("transient? should reject persistent collections")
	}
	assertPanics(t, "assoc! arity", func() {
		procAssocBang([]coretypes.Object{coretypes.MapToTransient(nil), coretypes.MakeKeyword(STRINGS.Intern, "k")})
	})
	assertPanics(t, "conj! map arity", func() {
		procConjBang([]coretypes.Object{coretypes.MapToTransient(nil), coretypes.MakeKeyword(STRINGS.Intern, "k")})
	})
}

func assertPanics(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s did not panic", name)
		}
	}()
	f()
}

func TestSeqContract(t *testing.T) {
	base := corecollections.NewListFrom(coretypes.MakeInt(1), coretypes.MakeInt(2), coretypes.MakeInt(3)).Seq()
	seqs := []struct {
		name string
		seq  coretypes.Seq
	}{
		{name: "list", seq: base},
		{name: "array", seq: &corecollections.ArraySeq{Arr: []coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(2), coretypes.MakeInt(3)}}},
		{name: "vector", seq: corecollections.NewVectorFrom(coretypes.MakeInt(1), coretypes.MakeInt(2), coretypes.MakeInt(3)).Seq()},
		{name: "cons", seq: corecollections.NewConsSeq(coretypes.MakeInt(1), corecollections.NewListFrom(coretypes.MakeInt(2), coretypes.MakeInt(3)).Seq())},
		{name: "take", seq: (&TakeSeq{seq: base, n: 3}).Seq()},
		{name: "filter", seq: (&FilteringSeq{seq: base, pred: Proc{Fn: func(args []coretypes.Object) coretypes.Object { return coretypes.Boolean{B: true} }}}).Seq()},
	}
	for _, tc := range seqs {
		if tc.seq.IsEmpty() {
			t.Fatalf("%s seq unexpectedly empty", tc.name)
		}
		if !tc.seq.First().Equals(coretypes.MakeInt(1)) {
			t.Fatalf("%s First = %s, want 1", tc.name, tc.seq.First().ToString(false))
		}
		if got := corecollections.Second(tc.seq); !got.Equals(coretypes.MakeInt(2)) {
			t.Fatalf("%s corecollections.Second = %s, want 2", tc.name, got.ToString(false))
		}
		if got := corecollections.Third(tc.seq); !got.Equals(coretypes.MakeInt(3)) {
			t.Fatalf("%s corecollections.Third = %s, want 3", tc.name, got.ToString(false))
		}
		if got := corecollections.SeqCount(tc.seq); got != 3 {
			t.Fatalf("%s corecollections.SeqCount = %d, want 3", tc.name, got)
		}
		if got := corecollections.SeqNth(tc.seq, 2); !got.Equals(coretypes.MakeInt(3)) {
			t.Fatalf("%s corecollections.SeqNth(2) = %s, want 3", tc.name, got.ToString(false))
		}
		if !coretypes.SeqsEqual(tc.seq, base) || !tc.seq.Equals(base) || tc.seq.Hash() != base.Hash() {
			t.Fatalf("%s should equal/hash like base sequence", tc.name)
		}
		withHead := tc.seq.Cons(coretypes.MakeInt(0))
		if corecollections.SeqCount(withHead) != 4 || !withHead.First().Equals(coretypes.MakeInt(0)) || !corecollections.Second(withHead).Equals(coretypes.MakeInt(1)) {
			t.Fatalf("%s Cons contract failed: %s", tc.name, withHead.ToString(false))
		}
	}
	if !corecollections.EmptyList.IsEmpty() || corecollections.SeqCount(corecollections.EmptyList) != 0 || !corecollections.EmptyList.Rest().IsEmpty() {
		t.Fatal("empty list sequence contract failed")
	}
	assertPanics(t, "negative corecollections.SeqNth", func() { corecollections.SeqNth(base, -1) })
}

func TestInfoAndMetaContract(t *testing.T) {
	info := &coretypes.ObjectInfo{Position: coretypes.Position{StartLine: 42}}
	meta := corecollections.EmptyArrayMap().Assoc(coretypes.MakeKeyword(STRINGS.Intern, "doc"), coretypes.MakeString("sample")).(coretypes.Map)
	values := []coretypes.Object{
		corecollections.NewArrayVectorFrom(coretypes.MakeInt(1)),
		corecollections.NewVectorFrom(coretypes.MakeInt(1)),
		corecollections.PersistentVectorFrom([]coretypes.Object{coretypes.MakeInt(1)}),
	}
	for _, v := range values {
		withInfo := coretypes.WithInfo(v, info)
		if withInfo.GetInfo() != info {
			t.Fatalf("%T WithInfo did not retain info", v)
		}
		withMeta, ok := withInfo.(coretypes.Meta)
		if !ok {
			t.Fatalf("%T does not implement coretypes.Meta after WithInfo", withInfo)
		}
		updated := withMeta.WithMeta(meta).(coretypes.Meta)
		if found, got := updated.GetMeta().Get(coretypes.MakeKeyword(STRINGS.Intern, "doc")); !found || !got.Equals(coretypes.MakeString("sample")) {
			t.Fatalf("%T WithMeta did not retain metadata", v)
		}
		if originalMeta, ok := v.(coretypes.Meta); ok && originalMeta.GetMeta() != nil {
			if found, _ := originalMeta.GetMeta().Get(coretypes.MakeKeyword(STRINGS.Intern, "doc")); found {
				t.Fatalf("%T WithMeta mutated original metadata", v)
			}
		}
	}
}

// ---- optimization_regression_test.go ----
func compileTestExpr(tb testing.TB, script string) Expr {
	tb.Helper()
	reader := NewReader(strings.NewReader(script), "<test>")
	obj, err := TryRead(reader)
	if err != nil {
		tb.Fatalf("read script: %v", err)
	}
	if _, err := TryRead(reader); err != io.EOF {
		tb.Fatalf("test script must contain exactly one form")
	}
	expr, err := TryParse(obj, &ParseContext{GlobalEnv: GLOBAL_ENV})
	if err != nil {
		tb.Fatalf("parse script: %v", err)
	}
	return expr
}

func evalTestScript(tb testing.TB, script string) coretypes.Object {
	tb.Helper()
	return Eval(compileTestExpr(tb, script), nil)
}

func requireInt(tb testing.TB, obj coretypes.Object, want int) {
	tb.Helper()
	got, ok := obj.(coretypes.Int)
	if !ok {
		tb.Fatalf("expected Int(%d), got %T (%s)", want, obj, obj.ToString(false))
	}
	if got.I != want {
		tb.Fatalf("expected Int(%d), got Int(%d)", want, got.I)
	}
}

func requireDouble(tb testing.TB, obj coretypes.Object, want float64) {
	tb.Helper()
	got, ok := obj.(coretypes.Double)
	if !ok {
		tb.Fatalf("expected Double(%v), got %T (%s)", want, obj, obj.ToString(false))
	}
	if got.D != want {
		tb.Fatalf("expected Double(%v), got Double(%v)", want, got.D)
	}
}

func requireBool(tb testing.TB, obj coretypes.Object, want bool) {
	tb.Helper()
	got, ok := obj.(coretypes.Boolean)
	if !ok {
		tb.Fatalf("expected Boolean(%v), got %T (%s)", want, obj, obj.ToString(false))
	}
	if got.B != want {
		tb.Fatalf("expected Boolean(%v), got Boolean(%v)", want, got.B)
	}
}

func requireString(tb testing.TB, obj coretypes.Object, want string) {
	tb.Helper()
	got, ok := obj.(coretypes.String)
	if !ok {
		tb.Fatalf("expected String(%q), got %T (%s)", want, obj, obj.ToString(false))
	}
	if got.S != want {
		tb.Fatalf("expected String(%q), got String(%q)", want, got.S)
	}
}

func TestIRInlineSlotCollision(t *testing.T) {
	// Regression test: inlined helper parameter must not collide with
	// capture slots or loop bindings when the fn's parameter frame
	// matches the caller's loop frame.
	t.Setenv("JOKER_IR_INLINE", "force")

	// sq's x param was at {frame:1, Index:0} — same as loop var i
	requireInt(t, evalTestScript(t, `(let [sq (fn [x] (* x x))
	              v [10 20 30]]
	  (loop [i 0 s 0]
	    (if (= i 3) s
	      (recur (inc i) (+ s (sq (nth v i)))))))`), 1400)

	// Two-arg helper with both params colliding
	requireInt(t, evalTestScript(t, `(let [f (fn [a b] (+ (* a a) b))
	              v [1 2 3 4 5]]
	  (loop [i 0 s 0]
	    (if (= i 5) s
	      (recur (inc i) (+ s (f (nth v i) i))))))`),
		// f(1,0)+f(2,1)+f(3,2)+f(4,3)+f(5,4) = 1+5+11+19+29 = 65
		65)
}

func TestIRInlineSmallHelper(t *testing.T) {
	t.Setenv("JOKER_IR_INLINE", "1")
	requireInt(t, evalTestScript(t, `(let [f (fn [x] (+ x 1))]
  (loop [i 0 acc 0]
    (if (= i 4)
      acc
      (recur (inc i) (+ acc (f i))))))`), 10)
}

func TestIRInlineCollectionHelper(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [pick (fn [v i] (+ (nth v i) 1))
                                      xs [1 2 3 4]]
  (loop [i 0 acc 0]
    (if (= i 4)
      acc
      (recur (inc i) (+ acc (pick xs i))))))`), 14)
}

func TestIRLetCaptureSlotCollisionRegression(t *testing.T) {
	// The inner let value captures outer ks, which used to allocate the same
	// IR slot as the let binding k. That collision disabled IR compilation for
	// map-update style loops and kept them on the slower tree-walker path.
	expr := compileTestExpr(t, `(let [ks [:k0 :k1 :k2 :k3]]
  (loop [i 0 m {}]
    (if (= i 20)
      (+ (get m :k0 0) (get m :k1 0))
      (let [k (nth ks (rem i 4))]
        (recur (inc i) (assoc m k (+ 1 (get m k 0))))))))`)
	let := expr.(*LetExpr)
	loop := let.body[0].(*LoopExpr)
	if prog := irCompile(loop); prog == nil {
		t.Fatal("expected IR compilation for captured inner let")
	}
	requireInt(t, Eval(expr, nil), 10)
}

func TestIRNestedLoopCaptureSlotCollisionRegression(t *testing.T) {
	expr := compileTestExpr(t, `(let [xs [1 2 3]]
  (loop [i 0 acc 0]
    (if (= i 3)
      acc
      (let [x (nth xs i)]
        (recur (inc i)
               (+ acc (loop [j 0 sum 0]
                        (if (= j x)
                          sum
                          (recur (inc j) (+ sum x))))))))))`)
	let := expr.(*LetExpr)
	loop := let.body[0].(*LoopExpr)
	if prog := irCompile(loop); prog == nil {
		t.Fatal("expected IR compilation for nested loop with captured init")
	}
	requireInt(t, Eval(expr, nil), 14)
}

func TestOptimizationRegressionArithmeticFastPaths(t *testing.T) {
	t.Run("int arithmetic", func(t *testing.T) {
		requireInt(t, evalTestScript(t, `(+ (* 6 7) (- 10 3))`), 49)
	})

	t.Run("mixed int double arithmetic", func(t *testing.T) {
		requireDouble(t, evalTestScript(t, `(+ (* 2.5 4) (- 7 2.5))`), 14.5)
	})

	t.Run("unary operations and predicates", func(t *testing.T) {
		requireBool(t, evalTestScript(t, `(zero? (- (inc 0) (dec 2)))`), true)
		requireInt(t, evalTestScript(t, `(- 7)`), -7)
	})

	t.Run("remainder", func(t *testing.T) {
		requireInt(t, evalTestScript(t, `(rem 29 5)`), 4)
	})
}

func TestOptimizationRegressionLoopAndBindingResolution(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [x 10]
  (loop [i 5 acc 0]
    (if (zero? i)
      (+ acc x)
      (recur (dec i) (+ acc x)))))`), 60)
}

func TestOptimizationRegressionRecursiveFib(t *testing.T) {
	requireInt(t, evalTestScript(t, `(letfn [(fib [n] (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2)))))]
  (fib 10))`), 55)
}

func TestOptimizationRegressionSeqNthAndTryNth(t *testing.T) {
	requireString(t, evalTestScript(t, `(nth (seq ["a" "b" "c" "d"]) 2)`), "c")
	requireString(t, evalTestScript(t, `(nth (seq ["a" "b"]) 5 "missing")`), "missing")
}

func TestOptimizationRegressionArraySeqDirectIndexing(t *testing.T) {
	requireString(t, evalTestScript(t, `(let [s (seq ["zero" "one" "two" "three"])]
  (str (nth s 0) "/" (nth s 2)))`), "zero/two")
}

func TestOptimizationRegressionMapGetAssoc(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [m (assoc (assoc {} :a 1) :b 2)]
  (+ (get m :a 0) (get m :b 0) (get m :missing 3)))`), 6)
}

func TestFrequenciesFastStringSeq(t *testing.T) {
	res := evalTestScript(t, `(frequencies ["alpha" "beta" "alpha" "theta" "beta" "alpha"])`)
	m, ok := res.(coretypes.Map)
	if !ok {
		t.Fatalf("expected map, got %T", res)
	}
	ok, v := m.Get(coretypes.String{S: "alpha"})
	if !ok || v.(coretypes.Int).I != 3 {
		t.Fatalf("expected alpha=3, got %v %v", ok, v)
	}
	ok, v = m.Get(coretypes.String{S: "theta"})
	if !ok || v.(coretypes.Int).I != 1 {
		t.Fatalf("expected theta=1, got %v %v", ok, v)
	}
}

func TestSplitWhitespace(t *testing.T) {
	res := evalTestScript(t, `(split-whitespace " alpha\tbeta  gamma\n")`)
	v, ok := res.(*corecollections.ArrayVector)
	if !ok {
		t.Fatalf("expected vector, got %T", res)
	}
	if v.Count() != 3 || v.At(0).(coretypes.String).S != "alpha" || v.At(2).(coretypes.String).S != "gamma" {
		t.Fatalf("unexpected split result: %s", v.ToString(false))
	}
}

func TestGeneratedCoreNamespacesDriveCoreNamespaceVar(t *testing.T) {
	ProcessCoreData()
	vr := GLOBAL_ENV.CoreNamespace.Resolve("*core-namespaces*")
	if vr == nil {
		t.Fatal("*core-namespaces* var not found")
	}
	set, ok := vr.Value.(*corecollections.MapSet)
	if !ok {
		t.Fatalf("*core-namespaces* = %T, want *corecollections.MapSet", vr.Value)
	}
	for _, ns := range coregenerated.CoreNamespaces() {
		if found, _ := set.Get(coretypes.MakeSymbol(STRINGS.Intern, ns)); !found {
			t.Fatalf("*core-namespaces* missing generated namespace %s", ns)
		}
	}
	if found, _ := set.Get(coretypes.MakeSymbol(STRINGS.Intern, "user")); !found {
		t.Fatal("*core-namespaces* missing user namespace")
	}
}

// ---- persistent_vector_test.go ----
func TestPVObjectSemantics(t *testing.T) {
	pv := corecollections.PersistentVectorFrom([]coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(2), coretypes.MakeInt(3)})
	av := corecollections.NewArrayVectorFrom(coretypes.MakeInt(1), coretypes.MakeInt(2), coretypes.MakeInt(3))
	if got := pv.ToString(false); got != "[1 2 3]" {
		t.Fatalf("ToString = %q", got)
	}
	if !pv.Equals(av) || !av.Equals(pv) {
		t.Fatal("persistent vector should compare equal to other counted/indexed vectors")
	}
	if pv.Hash() != av.Hash() {
		t.Fatalf("hash mismatch: persistent=%d array=%d", pv.Hash(), av.Hash())
	}
	if !pv.At(1).Equals(coretypes.MakeInt(2)) {
		t.Fatalf("At(1) = %s", pv.At(1).ToString(false))
	}
	if pv.Seq().IsEmpty() || !pv.Seq().First().Equals(coretypes.MakeInt(1)) {
		t.Fatal("coretypes.Seq did not expose first element")
	}
}

func TestPVEmpty(t *testing.T) {
	pv := corecollections.EmptyPersistentVector()
	if pv.Count() != 0 {
		t.Fatalf("expected count 0, got %d", pv.Count())
	}
}

func TestPVConjSmall(t *testing.T) {
	pv := corecollections.EmptyPersistentVector()
	for i := 0; i < 10; i++ {
		pv = pv.Conjoin(coretypes.MakeInt(i))
	}
	if pv.Count() != 10 {
		t.Fatalf("expected 10, got %d", pv.Count())
	}
	for i := 0; i < 10; i++ {
		v := pv.Nth(i).(coretypes.Int).I
		if v != i {
			t.Fatalf("Nth(%d) = %d, want %d", i, v, i)
		}
	}
}

func TestPVConjBeyondTail(t *testing.T) {
	pv := corecollections.EmptyPersistentVector()
	for i := 0; i < 100; i++ {
		pv = pv.Conjoin(coretypes.MakeInt(i))
	}
	if pv.Count() != 100 {
		t.Fatalf("expected 100, got %d", pv.Count())
	}
	for i := 0; i < 100; i++ {
		v := pv.Nth(i).(coretypes.Int).I
		if v != i {
			t.Fatalf("Nth(%d) = %d, want %d", i, v, i)
		}
	}
}

func TestPVConjLarge(t *testing.T) {
	pv := corecollections.EmptyPersistentVector()
	n := 2000
	for i := 0; i < n; i++ {
		pv = pv.Conjoin(coretypes.MakeInt(i))
	}
	if pv.Count() != n {
		t.Fatalf("expected %d, got %d", n, pv.Count())
	}
	for i := 0; i < n; i++ {
		v := pv.Nth(i).(coretypes.Int).I
		if v != i {
			t.Fatalf("Nth(%d) = %d, want %d", i, v, i)
		}
	}
}

func TestPVAssocTail(t *testing.T) {
	pv := corecollections.PersistentVectorFrom([]coretypes.Object{coretypes.MakeInt(0), coretypes.MakeInt(1), coretypes.MakeInt(2)})
	pv2 := pv.AssocIndex(1, coretypes.MakeInt(99))
	// Original unchanged
	if pv.Nth(1).(coretypes.Int).I != 1 {
		t.Fatal("original modified")
	}
	// New version has update
	if pv2.Nth(1).(coretypes.Int).I != 99 {
		t.Fatalf("assoc failed: got %d", pv2.Nth(1).(coretypes.Int).I)
	}
	// Other elements unchanged
	if pv2.Nth(0).(coretypes.Int).I != 0 || pv2.Nth(2).(coretypes.Int).I != 2 {
		t.Fatal("assoc corrupted other elements")
	}
}

func TestPVAssocInTrie(t *testing.T) {
	// Build a vector larger than 32 elements
	pv := corecollections.EmptyPersistentVector()
	for i := 0; i < 50; i++ {
		pv = pv.Conjoin(coretypes.MakeInt(i))
	}
	// Assoc in the trie portion (index < 32)
	pv2 := pv.AssocIndex(10, coretypes.MakeInt(999))
	if pv.Nth(10).(coretypes.Int).I != 10 {
		t.Fatal("original modified")
	}
	if pv2.Nth(10).(coretypes.Int).I != 999 {
		t.Fatalf("trie assoc failed: got %d", pv2.Nth(10).(coretypes.Int).I)
	}
	// Check structural sharing: other elements unchanged
	for i := 0; i < 50; i++ {
		if i == 10 {
			continue
		}
		if pv2.Nth(i).(coretypes.Int).I != i {
			t.Fatalf("structural sharing broken at %d", i)
		}
	}
}

func TestPVAssocAtEnd(t *testing.T) {
	pv := corecollections.PersistentVectorFrom([]coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(2), coretypes.MakeInt(3)})
	// Assoc at count = conj
	pv2 := pv.AssocIndex(3, coretypes.MakeInt(4))
	if pv2.Count() != 4 {
		t.Fatalf("expected 4, got %d", pv2.Count())
	}
	if pv2.Nth(3).(coretypes.Int).I != 4 {
		t.Fatal("assoc at end failed")
	}
}

func TestPVStructuralSharing(t *testing.T) {
	// Build vector, create two versions via assoc
	pv := corecollections.EmptyPersistentVector()
	for i := 0; i < 64; i++ {
		pv = pv.Conjoin(coretypes.MakeInt(i))
	}
	v1 := pv.AssocIndex(5, coretypes.MakeInt(500))
	v2 := pv.AssocIndex(40, coretypes.MakeInt(400))

	// All three versions are independent
	if pv.Nth(5).(coretypes.Int).I != 5 {
		t.Fatal("original corrupted")
	}
	if v1.Nth(5).(coretypes.Int).I != 500 {
		t.Fatal("v1 wrong")
	}
	if v2.Nth(5).(coretypes.Int).I != 5 {
		t.Fatal("v2 corrupted v1's change")
	}
	if v2.Nth(40).(coretypes.Int).I != 400 {
		t.Fatal("v2 wrong")
	}
	if v1.Nth(40).(coretypes.Int).I != 40 {
		t.Fatal("v1 corrupted by v2")
	}
}

func TestPVPop(t *testing.T) {
	pv := corecollections.PersistentVectorFrom([]coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(2), coretypes.MakeInt(3)})
	pv2 := pv.Pop().(*corecollections.PersistentVector)
	if pv2.Count() != 2 {
		t.Fatalf("expected 2, got %d", pv2.Count())
	}
	if pv2.Nth(0).(coretypes.Int).I != 1 || pv2.Nth(1).(coretypes.Int).I != 2 {
		t.Fatal("pop corrupted remaining elements")
	}
	if pv.Count() != 3 {
		t.Fatal("original modified by pop")
	}
}

func TestPVPopLarge(t *testing.T) {
	pv := corecollections.EmptyPersistentVector()
	for i := 0; i < 100; i++ {
		pv = pv.Conjoin(coretypes.MakeInt(i))
	}
	for i := 99; i >= 0; i-- {
		if pv.Count() != i+1 {
			t.Fatalf("count mismatch at pop %d: got %d", 100-i, pv.Count())
		}
		if pv.Nth(i).(coretypes.Int).I != i {
			t.Fatalf("last element wrong before pop %d", 100-i)
		}
		pv = pv.Pop().(*corecollections.PersistentVector)
	}
	if pv.Count() != 0 {
		t.Fatal("not empty after all pops")
	}
}

func TestPVToSlice(t *testing.T) {
	items := []coretypes.Object{coretypes.MakeInt(10), coretypes.MakeInt(20), coretypes.MakeInt(30)}
	pv := corecollections.PersistentVectorFrom(items)
	s := pv.ToSlice()
	if len(s) != 3 {
		t.Fatalf("expected 3, got %d", len(s))
	}
	for i, item := range items {
		if !s[i].Equals(item) {
			t.Fatalf("slice[%d] = %v, want %v", i, s[i], item)
		}
	}
}

func TestPVNthOutOfBounds(t *testing.T) {
	pv := corecollections.PersistentVectorFrom([]coretypes.Object{coretypes.MakeInt(1)})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for out-of-bounds nth")
		}
	}()
	pv.Nth(5)
}

func TestPVAssocOutOfBounds(t *testing.T) {
	pv := corecollections.PersistentVectorFrom([]coretypes.Object{coretypes.MakeInt(1)})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for out-of-bounds assoc")
		}
	}()
	pv.AssocIndex(5, coretypes.MakeInt(99))
}

func TestPVMultipleAssocSameBase(t *testing.T) {
	// Simulate n-body pattern: multiple assocs on same base vector
	pv := corecollections.EmptyPersistentVector()
	for i := 0; i < 35; i++ {
		pv = pv.Conjoin(coretypes.Double{D: float64(i)})
	}
	// Assoc 3 consecutive indices (like setting vx, vy, vz)
	v1 := pv.AssocIndex(3, coretypes.Double{D: 100.0})
	v2 := v1.AssocIndex(4, coretypes.Double{D: 200.0})
	v3 := v2.AssocIndex(5, coretypes.Double{D: 300.0})

	// Original unchanged
	if pv.Nth(3).(coretypes.Double).D != 3.0 {
		t.Fatal("original corrupted")
	}
	// Final has all three updates
	if v3.Nth(3).(coretypes.Double).D != 100.0 || v3.Nth(4).(coretypes.Double).D != 200.0 || v3.Nth(5).(coretypes.Double).D != 300.0 {
		t.Fatal("chained assoc failed")
	}
}

// Benchmark: PersistentVector assoc vs corecollections.ArrayVector assoc

// ---- program_analysis_test.go ----
func TestIRAnalysisNumericWASMPath(t *testing.T) {
	d := explainFirstLoop(compileTestExpr(t, `(loop [i 0 acc 0]
  (if (= i 10) acc (recur (inc i) (+ acc i))))`))
	if !d.Compiled {
		t.Fatalf("expected IR: %s", d.Reason)
	}
	if d.Analysis.SuggestedPath != "wasm" {
		t.Fatalf("expected wasm path, got %+v", d.Analysis)
	}
}

func TestIRAnalysisStringPrependBuilderPath(t *testing.T) {
	d := explainFirstLoop(compileTestExpr(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4) s (recur (inc i) (str (nth dna i) s)))))`))
	if !d.Compiled {
		t.Fatalf("expected IR: %s", d.Reason)
	}
	if !d.Analysis.HasStringPrepend || d.Analysis.SuggestedPath != "ir-string-prepend-builder" {
		t.Fatalf("expected prepend builder analysis, got %+v", d.Analysis)
	}
}

func TestIRAnalysisCollectionPath(t *testing.T) {
	d := explainFirstLoop(compileTestExpr(t, `(loop [i 0 m {}]
  (if (= i 4) (get m :a 0) (recur (inc i) (assoc m :a i))))`))
	if !d.Compiled {
		t.Fatalf("expected IR: %s", d.Reason)
	}
	if !d.Analysis.UsesCollection {
		t.Fatalf("expected collection analysis, got %+v", d.Analysis)
	}
}

func TestIRConstantCountFoldsStringBinding(t *testing.T) {
	expr := compileTestExpr(t, `(let [s "abcdef"]
  (loop [i 0 acc 0]
    (if (= i 3)
      acc
      (recur (inc i) (+ acc (count s))))))`)
	letExpr := expr.(*LetExpr)
	prog := irCompile(letExpr.body[0].(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	for pc := 0; pc < len(prog.code); pc++ {
		if prog.code[pc] == irCount {
			t.Fatal("expected count to be folded")
		}
	}
	requireInt(t, Eval(expr, nil), 18)
}

// ---- reader_construction_contract_test.go ----
func readOneForContract(t *testing.T, src string) coretypes.Object {
	t.Helper()
	r := NewReader(strings.NewReader(src), "<reader-contract>")
	obj, err := TryRead(r)
	if err != nil {
		t.Fatalf("TryRead(%q): %v", src, err)
	}
	if _, err := TryRead(r); err != io.EOF {
		t.Fatalf("TryRead(%q) should have exactly one form, next err=%v", src, err)
	}
	return obj
}

func requireReadErrorForContract(t *testing.T, src string) {
	t.Helper()
	r := NewReader(strings.NewReader(src), "<reader-contract>")
	if obj, err := TryRead(r); err == nil {
		t.Fatalf("TryRead(%q) = %s, want read error", src, obj.ToString(false))
	}
}

func TestReaderConstructionAdapterReaderSurface(t *testing.T) {
	adapter := ReaderConstructionAdapter{}
	reader := adapter.NewReader(strings.NewReader("[1 2]"), "<adapter>")
	obj, err := adapter.TryRead(reader)
	if err != nil {
		t.Fatalf("TryRead via adapter: %v", err)
	}
	vec := obj.(coretypes.CountedIndexed)
	if vec.Count() != 2 || !vec.At(0).Equals(coretypes.MakeInt(1)) || obj.GetInfo().FilenameOrUnknown() != "<adapter>" {
		t.Fatalf("adapter reader result mismatch: %s info=%#v", obj.ToString(false), obj.GetInfo())
	}
	if _, err := adapter.TryRead(reader); err != io.EOF {
		t.Fatalf("adapter TryRead should reach EOF, got %v", err)
	}
}

func TestReaderConstructionAdapterExpressionSurface(t *testing.T) {
	adapter := ReaderConstructionAdapter{}
	obj := coretypes.MakeString("literal")
	lit := adapter.LiteralExpr(obj)
	if lit.obj != obj || lit.isSurrogate {
		t.Fatalf("LiteralExpr mismatch: %#v", lit)
	}
	surrogate := adapter.SurrogateExpr(obj)
	if surrogate.obj != obj || !surrogate.isSurrogate {
		t.Fatalf("SurrogateExpr mismatch: %#v", surrogate)
	}
	pos := coretypes.Position{StartLine: 1, StartColumn: 2, EndLine: 1, EndColumn: 3}
	vec := adapter.VectorExpr([]Expr{lit}, pos)
	if len(vec.v) != 1 || vec.Position != pos {
		t.Fatalf("VectorExpr mismatch: %#v", vec)
	}
	m := adapter.MapExpr(2, pos)
	if len(m.keys) != 2 || len(m.values) != 2 || m.Position != pos {
		t.Fatalf("MapExpr mismatch: %#v", m)
	}
	set := adapter.SetExpr(3, pos)
	if len(set.elements) != 3 || set.Position != pos {
		t.Fatalf("SetExpr mismatch: %#v", set)
	}
	setFrom := adapter.SetExprFrom([]Expr{lit}, pos)
	if len(setFrom.elements) != 1 || setFrom.elements[0] != lit || setFrom.Position != pos {
		t.Fatalf("SetExprFrom mismatch: %#v", setFrom)
	}
	mapFrom := adapter.MapExprFrom([]Expr{lit}, []Expr{surrogate}, pos)
	if len(mapFrom.keys) != 1 || mapFrom.keys[0] != lit || len(mapFrom.values) != 1 || mapFrom.values[0] != surrogate || mapFrom.Position != pos {
		t.Fatalf("MapExprFrom mismatch: %#v", mapFrom)
	}
}

func TestReadIntegerUsesNativeIntRange(t *testing.T) {
	got := readOneForContract(t, "1099511627776")
	if strconv.IntSize == 32 {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("32-bit integer literal type = %s, want coretypes.BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("64-bit integer literal type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestReaderConstructionContractPrimitivesAndCollections(t *testing.T) {
	cases := []struct {
		src  string
		want coretypes.Object
	}{
		{`nil`, NIL},
		{`true`, coretypes.Boolean{B: true}},
		{`42`, coretypes.MakeInt(42)},
		{`"hi"`, coretypes.MakeString("hi")},
		{`:kw`, coretypes.MakeKeyword(STRINGS.Intern, "kw")},
		{`sym`, coretypes.MakeSymbol(STRINGS.Intern, "sym")},
	}
	for _, tc := range cases {
		got := readOneForContract(t, tc.src)
		if !got.Equals(tc.want) {
			t.Fatalf("read %s = %s (%T), want %s (%T)", tc.src, got.ToString(false), got, tc.want.ToString(false), tc.want)
		}
	}

	vecObj := readOneForContract(t, `[1 :two "three"]`)
	if vecObj.GetInfo() == nil || vecObj.GetInfo().FilenameOrUnknown() != "<reader-contract>" {
		t.Fatalf("vector did not retain source Info: %#v", vecObj.GetInfo())
	}
	vec := vecObj.(coretypes.CountedIndexed)
	if vec.Count() != 3 || !vec.At(0).Equals(coretypes.MakeInt(1)) || !vec.At(1).Equals(coretypes.MakeKeyword(STRINGS.Intern, "two")) || !vec.At(2).Equals(coretypes.MakeString("three")) {
		t.Fatalf("vector construction mismatch: %s", vec.(coretypes.Object).ToString(false))
	}
	m := readOneForContract(t, `{:a 1 "b" 2}`).(coretypes.Map)
	if m.Count() != 2 {
		t.Fatalf("map count = %d, want 2", m.Count())
	}
	if ok, got := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "a")); !ok || !got.Equals(coretypes.MakeInt(1)) {
		t.Fatalf("map keyword entry = %v %v", ok, got)
	}
	set := readOneForContract(t, `#{3 1 2}`).(*corecollections.MapSet)
	if set.Count() != 3 {
		t.Fatalf("set count = %d, want 3", set.Count())
	}
	namespaced := readOneForContract(t, `#:contract{:a 1 :b 2}`).(coretypes.Map)
	if found, got := namespaced.Get(coretypes.MakeKeyword(STRINGS.Intern, "contract/a")); !found || !got.Equals(coretypes.MakeInt(1)) {
		t.Fatalf("namespaced map keyword entry = %v %v", found, got)
	}
	list := readOneForContract(t, `(1 2 3)`).(coretypes.Seq)
	if corecollections.SeqCount(list) != 3 || !list.First().Equals(coretypes.MakeInt(1)) || !corecollections.Third(list).Equals(coretypes.MakeInt(3)) {
		t.Fatalf("list construction mismatch: %s", list.ToString(false))
	}

	requireReadErrorForContract(t, `{:a 1 :a 2}`)
	requireReadErrorForContract(t, `#{1 1}`)
	requireReadErrorForContract(t, `{:a 1 :b}`)
	requireReadErrorForContract(t, `#:#?@(:clj [foo]){:a 1}`)
}

func TestReaderConstructionContractRejectsInvalidArgLiteral(t *testing.T) {
	if _, err := TryRead(NewReader(strings.NewReader(`#(%0)`), "<reader-contract>")); err == nil {
		t.Fatal("expected invalid %0 arg literal to fail")
	}
}

func TestReaderConstructionContractMetadataTaggedReadersAndConditionals(t *testing.T) {
	metaObj := readOneForContract(t, `^:private [1 2]`)
	meta, ok := metaObj.(coretypes.Meta)
	if !ok || meta.GetMeta() == nil {
		t.Fatalf("metadata reader should produce coretypes.Meta object: %T", metaObj)
	}
	if found, got := meta.GetMeta().Get(coretypes.MakeKeyword(STRINGS.Intern, "private")); !found || !got.Equals(coretypes.Boolean{B: true}) {
		t.Fatalf("metadata did not contain :private true: %v %v", found, got)
	}
	if metaObj.GetInfo() == nil || metaObj.GetInfo().FilenameOrUnknown() != "<reader-contract>" {
		t.Fatalf("metadata form did not preserve source Info: %#v", metaObj.GetInfo())
	}

	dataReadersVar := GLOBAL_ENV.CoreNamespace.Resolve("*data-readers*")
	if dataReadersVar == nil {
		t.Fatal("*data-readers* var not found")
	}
	oldDataReaders := dataReadersVar.Value
	readers := corecollections.EmptyArrayMap()
	readers.Add(coretypes.MakeSymbol(STRINGS.Intern, "contract/direct"), Proc{Name: "readerContractDirect", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return corecollections.NewArrayVectorFrom(coretypes.MakeKeyword(STRINGS.Intern, "direct"), args[0])
	}})
	dataReadersVar.Value = readers
	defer func() { dataReadersVar.Value = oldDataReaders }()

	direct := readOneForContract(t, `#contract/direct 7`).(coretypes.CountedIndexed)
	if direct.Count() != 2 || !direct.At(0).Equals(coretypes.MakeKeyword(STRINGS.Intern, "direct")) || !direct.At(1).Equals(coretypes.MakeInt(7)) {
		t.Fatalf("direct tagged reader mismatch: %s", direct.(coretypes.Object).ToString(false))
	}

	fallbackVar := GLOBAL_ENV.CoreNamespace.Resolve("*default-data-reader-fn*")
	if fallbackVar == nil {
		t.Fatal("*default-data-reader-fn* var not found")
	}
	oldFallback := fallbackVar.Value
	fallbackVar.Value = Proc{Name: "readerContractFallback", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		return corecollections.NewArrayVectorFrom(args[0], args[1])
	}}
	defer func() { fallbackVar.Value = oldFallback }()

	tagged := readOneForContract(t, `#contract/tag {:x 1}`).(coretypes.CountedIndexed)
	if tagged.Count() != 2 || !tagged.At(0).Equals(coretypes.MakeSymbol(STRINGS.Intern, "contract/tag")) {
		t.Fatalf("tagged fallback mismatch: %s", tagged.(coretypes.Object).ToString(false))
	}
	payload, ok := tagged.At(1).(coretypes.Map)
	if !ok {
		t.Fatalf("tagged fallback payload = %T, want coretypes.Map", tagged.At(1))
	}
	if found, got := payload.Get(coretypes.MakeKeyword(STRINGS.Intern, "x")); !found || !got.Equals(coretypes.MakeInt(1)) {
		t.Fatalf("tagged fallback payload entry = %v %v", found, got)
	}

	selected := readOneForContract(t, `#?(:missing :no :joker [1 2])`)
	selectedVec, ok := selected.(coretypes.CountedIndexed)
	if !ok || selectedVec.Count() != 2 || !selectedVec.At(0).Equals(coretypes.MakeInt(1)) || !selectedVec.At(1).Equals(coretypes.MakeInt(2)) {
		t.Fatalf("reader conditional selected wrong form: %s", selected.ToString(false))
	}
	spliced := readOneForContract(t, `(#?@(:missing [:no] :joker [1 2]) 3)`).(coretypes.Seq)
	if corecollections.SeqCount(spliced) != 3 || !spliced.First().Equals(coretypes.MakeInt(1)) || !corecollections.Second(spliced).Equals(coretypes.MakeInt(2)) || !corecollections.Third(spliced).Equals(coretypes.MakeInt(3)) {
		t.Fatalf("reader conditional splice mismatch: %s", spliced.ToString(false))
	}
}

func TestReadConditionalSpliceEmptyInList(t *testing.T) {
	reader := NewReader(strings.NewReader("(do #?@(:definitely-nope [1 2]) 3)"), "<test>")
	obj, err := TryRead(reader)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	lst, ok := obj.(*corecollections.List)
	if !ok {
		t.Fatalf("expected list, got %T", obj)
	}
	if got := lst.Count(); got != 2 {
		t.Fatalf("expected 2 elements (do, 3), got %d", got)
	}
}

func TestReadConditionalNestedSpliceNoRuntimePanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected runtime panic: %v", r)
		}
	}()

	reader := NewReader(strings.NewReader("#?(:x #?@(:x [1 2]))"), "<test>")
	_, err := TryRead(reader)
	if err == nil {
		t.Fatal("expected read error for invalid nested splice")
	}
	if !strings.Contains(err.Error(), "Read error") {
		t.Fatalf("expected read error, got: %v", err)
	}
}

func TestReaderConstructionAdapterReadObjectAndError(t *testing.T) {
	r := readerConstruction.NewReader(strings.NewReader("x"), "<adapter-contract>")
	pushPos(r)
	_ = r.Get()
	obj := readerConstruction.ReadObject(r, coretypes.MakeSymbol(STRINGS.Intern, "x"))
	info := obj.GetInfo()
	if info == nil || info.FilenameOrUnknown() != "<adapter-contract>" || info.StartLine != 1 || info.StartColumn != 0 || info.EndLine != 1 || info.EndColumn != 1 {
		t.Fatalf("adapter ReadObject info = %#v", info)
	}
	err := readerConstruction.ReadError(r, "boom")
	if !strings.Contains(err.Error(), "<adapter-contract>:1:1") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("adapter ReadError = %v", err)
	}
}

func TestReaderConstructionAdapterScalarObjects(t *testing.T) {
	if !readerConstruction.Nil().Equals(NIL) {
		t.Fatal("adapter Nil mismatch")
	}
	if !readerConstruction.Bool(true).Equals(coretypes.Boolean{B: true}) || !readerConstruction.Bool(false).Equals(coretypes.Boolean{B: false}) {
		t.Fatal("adapter Bool mismatch")
	}
	if !readerConstruction.Char('x').Equals(coretypes.Char{Ch: 'x'}) {
		t.Fatal("adapter Char mismatch")
	}
	if !readerConstruction.Int(7).Equals(coretypes.MakeInt(7)) {
		t.Fatal("adapter Int mismatch")
	}
	if !readerConstruction.Double(1.5).Equals(coretypes.MakeDouble(1.5)) {
		t.Fatal("adapter Double mismatch")
	}
	if c, ok := readerConstruction.Comment(";").(coretypes.Comment); !ok || c.C != ";" {
		t.Fatalf("adapter coretypes.Comment mismatch: %#v", c)
	}
	rxObj, ok := readerConstruction.Regex(regexp.MustCompile("x+")).(*coretypes.Regex)
	if !ok || rxObj.R == nil || !rxObj.R.MatchString("xxx") {
		t.Fatalf("adapter Regex mismatch: %#v", rxObj)
	}
	if !readerConstruction.String("x").Equals(coretypes.MakeString("x")) {
		t.Fatal("adapter String mismatch")
	}
	if !readerConstruction.Symbol("x").Equals(coretypes.MakeSymbol(STRINGS.Intern, "x")) {
		t.Fatal("adapter Symbol mismatch")
	}
	if !readerConstruction.Keyword("x").Equals(coretypes.MakeKeyword(STRINGS.Intern, "x")) {
		t.Fatal("adapter Keyword mismatch")
	}
}

func TestReaderConstructionAdapterSetLiteral(t *testing.T) {
	r := readerConstruction.NewReader(strings.NewReader("#{}"), "<adapter-contract>")
	pushPos(r)
	set := readerConstruction.SetLiteral(r, []coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(2)}).(*corecollections.MapSet)
	if set.Count() != 2 {
		t.Fatalf("SetLiteral count = %d, want 2", set.Count())
	}
	obj := readerConstruction.ReadObject(r, set)
	if obj.GetInfo() == nil || obj.GetInfo().FilenameOrUnknown() != "<adapter-contract>" {
		t.Fatalf("SetLiteral read object info = %#v", obj.GetInfo())
	}
}

func TestReaderConstructionAdapterMapLiteral(t *testing.T) {
	r := readerConstruction.NewReader(strings.NewReader("{}"), "<adapter-contract>")
	pushPos(r)
	m := readerConstruction.MapLiteral(r, []coretypes.Object{coretypes.MakeKeyword(STRINGS.Intern, "a"), coretypes.MakeInt(1)}, "").(coretypes.Map)
	if found, got := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "a")); !found || !got.Equals(coretypes.MakeInt(1)) {
		t.Fatalf("MapLiteral entry = %v %v", found, got)
	}
	obj := readerConstruction.ReadObject(r, m)
	if obj.GetInfo() == nil || obj.GetInfo().FilenameOrUnknown() != "<adapter-contract>" {
		t.Fatalf("MapLiteral read object info = %#v", obj.GetInfo())
	}
}

func TestReaderConstructionAdapterMetadata(t *testing.T) {
	meta, ok := readerConstruction.MetadataFromObject(coretypes.MakeKeyword(STRINGS.Intern, "private"))
	if !ok {
		t.Fatal("keyword metadata not accepted")
	}
	if found, got := meta.Get(coretypes.MakeKeyword(STRINGS.Intern, "private")); !found || !got.Equals(coretypes.Boolean{B: true}) {
		t.Fatalf("keyword metadata entry = %v %v", found, got)
	}
	vec := corecollections.NewArrayVectorFrom(coretypes.MakeInt(1))
	withMeta, ok := readerConstruction.WithMeta(vec, meta)
	if !ok || withMeta.(coretypes.Meta).GetMeta() == nil {
		t.Fatalf("WithMeta = %T %v", withMeta, ok)
	}
	if _, ok := readerConstruction.MetadataFromObject(coretypes.MakeInt(1)); ok {
		t.Fatal("integer metadata accepted")
	}
	if _, ok := readerConstruction.WithMeta(coretypes.MakeInt(1), meta); ok {
		t.Fatal("metadata applied to int")
	}
	skip := readerConstruction.SkipRedundantDoMeta()
	if found, got := skip.Get(coretypes.MakeKeyword(STRINGS.Intern, "skip-redundant-do")); !found || !got.Equals(coretypes.Boolean{B: true}) {
		t.Fatalf("SkipRedundantDoMeta entry = %v %v", found, got)
	}
}

func TestReaderConstructionAdapterNumericObjects(t *testing.T) {
	bi := readerConstruction.BigInt(big.NewInt(42), "42")
	if bi.GetType() != TYPE.BigInt || bi.ToString(false) != "42N" {
		t.Fatalf("adapter coretypes.BigInt = %s type=%s", bi.ToString(false), bi.GetType().ToString(false))
	}
	bf, ok := readerConstruction.BigFloatFromString("1.25", "1.25M")
	if !ok || bf.GetType() != TYPE.BigFloat {
		t.Fatalf("adapter coretypes.BigFloat = %v %T", ok, bf)
	}
	r := readerConstruction.RatioOrInt("2/4", big.NewRat(2, 4))
	if !r.Equals(coretypes.MakeInt(0)) && r.GetType() != TYPE.Ratio && r.GetType() != TYPE.Int {
		t.Fatalf("adapter RatioOrInt unexpected: %s type=%s", r.ToString(false), r.GetType().ToString(false))
	}
}

func TestReaderConstructionAdapterNumberFromToken(t *testing.T) {
	r := readerConstruction.NewReader(strings.NewReader("42"), "<adapter-contract>")
	pushPos(r)
	_ = r.Get()
	n := readerConstruction.NumberFromToken(r, corereader.NumberToken{Kind: corereader.NumberTokenInt, Original: "42", Digits: "42", Base: 10})
	if !n.Equals(coretypes.MakeInt(42)) {
		t.Fatalf("adapter NumberFromToken = %s, want 42", n.ToString(false))
	}
}

func TestReaderConstructionAdapterCollectionObjects(t *testing.T) {
	list := readerConstruction.ListFrom([]coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(2)}).(coretypes.Seq)
	if corecollections.SeqCount(list) != 2 || !list.First().Equals(coretypes.MakeInt(1)) || !corecollections.Second(list).Equals(coretypes.MakeInt(2)) {
		t.Fatalf("adapter ListFrom mismatch: %s", list.ToString(false))
	}
	vec := readerConstruction.VectorFrom([]coretypes.Object{coretypes.MakeKeyword(STRINGS.Intern, "a"), coretypes.MakeKeyword(STRINGS.Intern, "b")}).(coretypes.CountedIndexed)
	if vec.Count() != 2 || !vec.At(0).Equals(coretypes.MakeKeyword(STRINGS.Intern, "a")) || !vec.At(1).Equals(coretypes.MakeKeyword(STRINGS.Intern, "b")) {
		t.Fatalf("adapter coretypes.VectorFrom mismatch: %s", vec.(coretypes.Object).ToString(false))
	}
	persistentObj := readerConstruction.PersistentVectorFromSeq(vec.(coretypes.Seqable).Seq())
	persistent, ok := persistentObj.(*corecollections.PersistentVector)
	if !ok {
		t.Fatalf("adapter PersistentVectorFromSeq type = %T, want *corecollections.PersistentVector", persistentObj)
	}
	if persistent.Count() != 2 || !persistent.At(0).Equals(coretypes.MakeKeyword(STRINGS.Intern, "a")) || !persistent.At(1).Equals(coretypes.MakeKeyword(STRINGS.Intern, "b")) {
		t.Fatalf("adapter PersistentVectorFromSeq mismatch: %s", persistent.ToString(false))
	}
	if readerConstruction.VectorFrom(nil).(coretypes.Counted).Count() != 0 {
		t.Fatal("adapter empty coretypes.VectorFrom not empty")
	}
}

func TestReaderConstructionAdapterDeriveReadObject(t *testing.T) {
	r := readerConstruction.NewReader(strings.NewReader("x"), "<adapter-contract>")
	pushPos(r)
	_ = r.Get()
	base := readerConstruction.ReadObject(r, coretypes.MakeSymbol(STRINGS.Intern, "x"))
	derived := readerConstruction.DeriveReadObject(base, coretypes.MakeKeyword(STRINGS.Intern, "x"))
	if derived.GetInfo() == nil || derived.GetInfo().FilenameOrUnknown() != "<adapter-contract>" {
		t.Fatalf("derived info = %#v", derived.GetInfo())
	}
}

// ---- runtime_execution_boundary_guard_test.go ----
func TestExecutorFilesUseRuntimeExecutionAdapterForProgramState(t *testing.T) {
	for _, file := range []string{
		"boxed_exec.go",
		"typed_exec.go",
		"typed_exec_inline.go",
		"typed_exec_nanbox.go",
	} {
		data, err := os.ReadFile(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for lineNo, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "prog.") {
				t.Fatalf("%s:%d reaches into IRProgram state instead of runtimeExec adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			if strings.Contains(line, "RuntimeExecutionAdapter{}") {
				t.Fatalf("%s:%d constructs ad-hoc runtime adapter instead of shared runtimeExec: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			if strings.Contains(line, "currentGRT()") || strings.Contains(line, ".Call(") || strings.Contains(line, "(coretypes.Callable)") {
				t.Fatalf("%s:%d performs call/runtime dispatch instead of runtimeExec adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			lineText := strings.TrimSpace(line)
			if lineText == "case *Fn:" || lineText == "return (*Fn)(v.p)" {
				continue
			}
			if strings.Contains(line, "*Fn") || strings.Contains(line, "irGetFnProg") || strings.Contains(line, "wasmGetFn") || strings.Contains(line, ".env") {
				t.Fatalf("%s:%d reaches into Fn internals instead of runtimeExec adapter: %s", file, lineNo+1, lineText)
			}
			if strings.Contains(line, ".Equals(") {
				t.Fatalf("%s:%d performs equality instead of runtimeExec adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			if strings.Contains(line, "corecollections.ToSlice(") || strings.Contains(line, "(coretypes.Seqable)") {
				t.Fatalf("%s:%d prepares call args instead of runtimeExec adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			lineText = strings.TrimSpace(line)
			if lineText == "case *corecollections.ArrayVector:" || lineText == "case *coretypes.TransientVector:" || lineText == "return (*corecollections.ArrayVector)(v.p)" || lineText == "return (*coretypes.TransientVector)(v.p)" {
				continue
			}
			if strings.Contains(line, "coretypes.Seqable") || strings.Contains(line, "coretypes.Conjable") || strings.Contains(line, "Counted") || strings.Contains(line, "coretypes.Associative") || strings.Contains(line, "*coretypes.TransientVector") || strings.Contains(line, "*corecollections.ArrayVector") || strings.Contains(line, "&corecollections.ArrayVector") {
				t.Fatalf("%s:%d performs collection construction/access instead of runtimeExec adapter: %s", file, lineNo+1, lineText)
			}
			if lineText == "case *corert.StringCursor:" || lineText == "return (*corert.StringCursor)(v.p)" {
				continue
			}
			if strings.Contains(line, "*corert.StringCursor") || strings.Contains(line, ".Char()") || strings.Contains(line, ".Next()") || strings.Contains(line, ".Done()") {
				t.Fatalf("%s:%d performs cursor access instead of runtimeExec adapter: %s", file, lineNo+1, lineText)
			}
		}
	}
}

// ---- runtime_execution_contract_test.go ----
func TestRuntimeExecutionAdapterEquality(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	if !adapter.Equal(coretypes.MakeInt(1), coretypes.MakeInt(1)) {
		t.Fatal("Equal rejected matching ints")
	}
	if adapter.Equal(coretypes.MakeInt(1), coretypes.MakeInt(2)) {
		t.Fatal("Equal accepted mismatched ints")
	}
}

func TestRuntimeExecutionAdapterPrepareCallSlotsInstallsCaptures(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	prog := &IRProgram{
		numSlots:        3,
		captureKeys:     []bindingKey{{index: 0}},
		captureSlotIdxs: []int{2},
	}
	env := &LocalEnv{bindings: []coretypes.Object{coretypes.MakeString("captured")}}
	args := []coretypes.Object{coretypes.MakeInt(1)}
	full := adapter.PrepareCallSlots(prog, args, env)
	if len(full) != 3 || full[0] != args[0] || full[2].(coretypes.String).S != "captured" {
		t.Fatalf("prepared call slots mismatch: %#v", full)
	}
	if got := adapter.PrepareCallSlots(&IRProgram{}, args, env); len(got) != 1 || got[0] != args[0] {
		t.Fatalf("capture-free call should reuse args: %#v", got)
	}
}

func TestRuntimeExecutionAdapterInstallsTypedEnvCaptures(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	objects := adapter.ObjectsFromTypedValues([]irValue{objectToIRValue(coretypes.MakeInt(1)), objectToIRValue(coretypes.MakeString("x"))}, make([]coretypes.Object, 2))
	if len(objects) != 2 || !objects[0].Equals(coretypes.MakeInt(1)) || !objects[1].Equals(coretypes.MakeString("x")) {
		t.Fatalf("ObjectsFromTypedValues = %#v", objects)
	}
	prog := &IRProgram{
		numSlots:        2,
		captureKeys:     []bindingKey{{index: 0}},
		captureSlotIdxs: []int{1},
	}
	env := &LocalEnv{bindings: []coretypes.Object{coretypes.MakeInt(42)}}
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
		constants:       []coretypes.Object{coretypes.MakeInt(7)},
		fnExprs:         []*FnExpr{fnExpr},
		captureSlotIdxs: []int{2},
		captureSlots:    []coretypes.Object{coretypes.MakeString("captured")},
	}
	if got := adapter.ProgramNumSlots(prog); got != 3 {
		t.Fatalf("ProgramNumSlots = %d, want 3", got)
	}
	if got := adapter.ProgramCode(prog); len(got) != 3 || got[0] != 1 {
		t.Fatalf("ProgramCode = %#v", got)
	}
	if got, ok := adapter.ProgramConstant(prog, 0); !ok || !got.Equals(coretypes.MakeInt(7)) {
		t.Fatalf("ProgramConstant = %#v, %v", got, ok)
	}
	if got := adapter.ProgramConstants(prog); len(got) != 1 || !got[0].Equals(coretypes.MakeInt(7)) {
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
	fnObj := &Fn{irProg: prog, env: &LocalEnv{bindings: []coretypes.Object{coretypes.MakeInt(1)}}}
	if got, ok := adapter.FnProgram(fnObj); !ok || got != prog {
		t.Fatalf("FnProgram = %#v, %v", got, ok)
	}
	failedFn := &Fn{irProg: irCompileFailed}
	if got, ok := adapter.FnProgram(failedFn); ok || got != nil {
		t.Fatalf("FnProgram should hide compile-failed sentinel, got %#v, %v", got, ok)
	}
	if slots, ok := adapter.FnCallSlots(fnObj, prog, []coretypes.Object{coretypes.MakeInt(2)}); !ok || len(slots) == 0 || !slots[0].Equals(coretypes.MakeInt(2)) {
		t.Fatalf("FnCallSlots = %#v, %v", slots, ok)
	}
	if !adapter.ProgramHasCaptureSlots(prog) {
		t.Fatal("ProgramHasCaptureSlots returned false")
	}
	objectSlots := []coretypes.Object{NIL, NIL, NIL}
	if !adapter.ApplyProgramCaptureSlots(prog, objectSlots) || !objectSlots[2].Equals(coretypes.MakeString("captured")) {
		t.Fatalf("ApplyProgramCaptureSlots = %#v", objectSlots)
	}
	typedSlots := make([]irValue, 3)
	if !adapter.ApplyProgramTypedCaptureSlots(prog, typedSlots) || !typedSlots[2].object().Equals(coretypes.MakeString("captured")) {
		t.Fatalf("ApplyProgramTypedCaptureSlots = %#v", typedSlots)
	}
	typedSlots[1] = objectToIRValue(coretypes.MakeInt(99))
	if !adapter.ClearTypedNonCaptureSlots(prog, typedSlots, 1) || !typedSlots[2].object().Equals(coretypes.MakeString("captured")) || typedSlots[1] != (irValue{}) {
		t.Fatalf("ClearTypedNonCaptureSlots = %#v", typedSlots)
	}
	prog.captureSlotSet = []bool{false}
	if adapter.ClearTypedNonCaptureSlots(prog, typedSlots, 1) {
		t.Fatal("ClearTypedNonCaptureSlots accepted short capture slot set")
	}
	prog.captureSlotSet = nil
	idxs, captures := adapter.ProgramCaptureSlots(prog)
	if len(idxs) != 1 || idxs[0] != 2 || len(captures) != 1 || !captures[0].Equals(coretypes.MakeString("captured")) {
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
	fnObj := adapter.MakeFn(fnExpr, []coretypes.Object{coretypes.MakeInt(10)})
	fn, ok := fnObj.(*Fn)
	if !ok {
		t.Fatalf("MakeFn returned %T, want *Fn", fnObj)
	}
	if fn.env == nil || len(fn.env.bindings) != 1 || !fn.env.bindings[0].Equals(coretypes.MakeInt(10)) {
		t.Fatalf("MakeFn did not capture slots: %#v", fn.env)
	}
}

func TestRuntimeExecutionAdapterCallObject(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	args, ok := adapter.CallArgs(corecollections.NewArrayVectorFrom(coretypes.MakeInt(1), coretypes.MakeInt(2)))
	if !ok || len(args) != 2 || !args[1].Equals(coretypes.MakeInt(2)) {
		t.Fatalf("CallArgs = %#v, %v", args, ok)
	}
	if _, ok := adapter.CallArgs(coretypes.MakeInt(1)); ok {
		t.Fatal("CallArgs accepted non-seqable")
	}
	fn := Proc{Name: "contract-call", Fn: func(args []coretypes.Object) coretypes.Object { return coretypes.MakeInt(len(args)) }}
	got, ok := adapter.CallObject(fn, []coretypes.Object{coretypes.MakeInt(1), coretypes.MakeInt(2)})
	if !ok || !got.Equals(coretypes.MakeInt(2)) {
		t.Fatalf("CallObject = %#v, %v", got, ok)
	}
	if _, ok := adapter.CallObject(coretypes.MakeInt(1), nil); ok {
		t.Fatal("CallObject accepted non-callable")
	}
	got, ok = adapter.CallObjectWithSyntheticCallExpr(fn, []coretypes.Object{coretypes.MakeInt(1)})
	if !ok || !got.Equals(coretypes.MakeInt(1)) {
		t.Fatalf("CallObjectWithSyntheticCallExpr = %#v, %v", got, ok)
	}
	grt := currentGRT()
	prevExpr := grt.CurrentExpr
	panicking := Proc{Name: "contract-panic", Fn: func(args []coretypes.Object) coretypes.Object { panic("boom") }}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("CallObjectWithSyntheticCallExpr did not propagate panic")
			}
		}()
		_, _ = adapter.CallObjectWithSyntheticCallExpr(panicking, nil)
	}()
	if grt.CurrentExpr != prevExpr {
		t.Fatal("CallObjectWithSyntheticCallExpr did not restore current expression after panic")
	}
}

func TestRuntimeExecutionAdapterCollectionOps(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	vec := corecollections.NewArrayVectorFrom(coretypes.MakeInt(1), coretypes.MakeInt(2))
	if got, ok := adapter.Nth(vec, 1); !ok || !got.Equals(coretypes.MakeInt(2)) {
		t.Fatalf("Nth = %#v, %v", got, ok)
	}
	if _, ok := adapter.Nth(vec, 9); ok {
		t.Fatal("Nth accepted out-of-range index")
	}
	mapObj := corecollections.EmptyArrayMap().Assoc(coretypes.MakeString("k"), coretypes.MakeInt(7)).(coretypes.Object)
	if got := adapter.Get(mapObj, coretypes.MakeString("k"), NIL); !got.Equals(coretypes.MakeInt(7)) {
		t.Fatalf("Get = %#v", got)
	}
	if got := adapter.Get(vec, coretypes.MakeInt(9), coretypes.MakeInt(42)); !got.Equals(coretypes.MakeInt(42)) {
		t.Fatalf("Get default = %#v", got)
	}
	if got, ok := adapter.Assoc(vec, coretypes.MakeInt(0), coretypes.MakeInt(9)); !ok {
		t.Fatalf("Assoc returned %#v, %v", got, ok)
	} else if ok, val := got.(coretypes.Gettable).Get(coretypes.MakeInt(0)); !ok || !val.Equals(coretypes.MakeInt(9)) {
		t.Fatalf("Assoc value = %#v, %v", val, ok)
	}
	if got, ok := adapter.Conj(vec, coretypes.MakeInt(3)); !ok || got.(coretypes.Counted).Count() != 3 {
		t.Fatalf("Conj returned %#v, %v", got, ok)
	}
	if got, ok := adapter.First(vec); !ok || !got.Equals(coretypes.MakeInt(1)) {
		t.Fatalf("First returned %#v, %v", got, ok)
	}
	if got, ok := adapter.First(corecollections.EmptyArrayVector()); !ok || got != NIL {
		t.Fatalf("First empty returned %#v, %v", got, ok)
	}
	if got := adapter.BuildVector([]coretypes.Object{coretypes.MakeInt(4), coretypes.MakeInt(5)}); got.(coretypes.Counted).Count() != 2 {
		t.Fatalf("BuildVector returned %#v", got)
	}
	transient := coretypes.ToTransient(vec.Arr)
	if !adapter.HasMutableSlotCandidate([]coretypes.Object{coretypes.MakeInt(1), vec}) {
		t.Fatal("HasMutableSlotCandidate missed vector")
	}
	if adapter.HasMutableSlotCandidate([]coretypes.Object{coretypes.MakeInt(1), coretypes.MakeSymbol(STRINGS.Intern, "x")}) {
		t.Fatal("HasMutableSlotCandidate should ignore non-candidate objects")
	}
	if got := adapter.MutableSlotObject(vec, &EscapeInfo{SafeMutableSlots: []bool{true}}, 0); got == vec {
		t.Fatalf("MutableSlotObject did not convert vector: %#v", got)
	}
	if got := adapter.PersistentResult(transient); got.(coretypes.Counted).Count() != 2 {
		t.Fatalf("PersistentResult = %#v", got)
	}
	transientObj, ok := adapter.ToTransient(vec)
	if !ok {
		t.Fatal("ToTransient rejected vector")
	}
	if got, ok := adapter.AssocBang(transientObj, coretypes.MakeInt(1), coretypes.MakeInt(8)); !ok {
		t.Fatalf("AssocBang returned %#v, %v", got, ok)
	} else if got, ok := adapter.ToPersistent(got); !ok || got.(coretypes.Indexed).Nth(1) != coretypes.MakeInt(8) {
		t.Fatalf("ToPersistent after AssocBang = %#v, %v", got, ok)
	}
}

func TestRuntimeExecutionAdapterStringOps(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	if got := adapter.Str1(coretypes.MakeChar('x')); !got.Equals(coretypes.MakeString("x")) {
		t.Fatalf("Str1 char = %#v", got)
	}
	if got := adapter.Str2(coretypes.MakeString("a"), coretypes.MakeChar('b')); !got.Equals(coretypes.MakeString("ab")) {
		t.Fatalf("Str2 = %#v", got)
	}
	if got, ok := adapter.Count(coretypes.MakeString("abc")); !ok || got != 3 {
		t.Fatalf("Count = %d, %v", got, ok)
	}
	prog := &IRProgram{constants: []coretypes.Object{coretypes.MakeString("abc")}}
	if got, ok := adapter.NthASCIIStringConst(prog, 0, 1); !ok || !got.Equals(coretypes.MakeChar('b')) {
		t.Fatalf("NthASCIIStringConst = %#v, %v", got, ok)
	}
	cur := corert.NewStringCursor("x")
	if got, ok := adapter.CursorChar(cur); !ok || !got.Equals(coretypes.MakeChar('x')) {
		t.Fatalf("CursorChar = %#v, %v", got, ok)
	}
	if got, ok := adapter.CursorDone(cur); !ok || got.(coretypes.Boolean).B {
		t.Fatalf("CursorDone = %#v, %v", got, ok)
	}
	if got, ok := adapter.CursorNext(cur); !ok || got == cur {
		t.Fatalf("CursorNext = %#v, %v", got, ok)
	}
}

func TestRuntimeExecutionAdapterErrorf(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	err := adapter.Errorf("contract %d", 42)
	msg, ok := err.Message().(coretypes.String)
	if err == nil || !ok || msg.S != "contract 42" {
		t.Fatalf("Errorf = %#v", err)
	}
}

func TestRuntimeExecutionAdapterApplyCaptureSlots(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	slots := []coretypes.Object{NIL, NIL, NIL}
	if !adapter.ApplyCaptureSlots(slots, []int{2, 0}, []coretypes.Object{coretypes.MakeInt(20), coretypes.MakeInt(10)}) {
		t.Fatal("ApplyCaptureSlots returned false for valid captures")
	}
	if !slots[0].Equals(coretypes.MakeInt(10)) || !slots[2].Equals(coretypes.MakeInt(20)) {
		t.Fatalf("capture slots = %#v", slots)
	}
	if adapter.ApplyCaptureSlots(slots, []int{3}, []coretypes.Object{coretypes.MakeInt(1)}) {
		t.Fatal("ApplyCaptureSlots accepted out-of-range slot")
	}
	if adapter.ApplyCaptureSlots(slots, []int{1, 2}, []coretypes.Object{coretypes.MakeInt(1)}) {
		t.Fatal("ApplyCaptureSlots accepted mismatched metadata")
	}
}

func TestRuntimeExecutionAdapterApplyTypedCaptureSlots(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	slots := make([]irValue, 2)
	if !adapter.ApplyTypedCaptureSlots(slots, []int{1}, []coretypes.Object{coretypes.MakeInt(42)}) {
		t.Fatal("ApplyTypedCaptureSlots returned false for valid captures")
	}
	if slots[1].tag != irValInt || slots[1].i != 42 {
		t.Fatalf("typed capture slot = %#v", slots[1])
	}
	if adapter.ApplyTypedCaptureSlots(slots, []int{-1}, []coretypes.Object{coretypes.MakeInt(1)}) {
		t.Fatal("ApplyTypedCaptureSlots accepted out-of-range slot")
	}
}

func TestRuntimeExecutionAdapterThrow(t *testing.T) {
	adapter := RuntimeExecutionAdapter{}
	defer func() {
		r := recover()
		err, ok := r.(coretypes.Error)
		if !ok {
			t.Fatalf("Throw panic = %T, want core coretypes.Error", r)
		}
		msg := err.Message().(coretypes.String)
		if msg.S != "boom" {
			t.Fatalf("Throw message = %q, want boom", msg.S)
		}
	}()
	adapter.Throw(coretypes.MakeString("boom"))
}

func TestIRCompileFailureIsCachedOnFn(t *testing.T) {
	fn := evalTestScript(t, `(fn [x] (println x))`).(*Fn)
	if prog := irGetFnProg(fn); prog != nil {
		t.Fatalf("string-building fn unexpectedly compiled to IR: %#v", prog)
	}
	if atomic.LoadUint32(&fn.irProgOnce) != 1 {
		t.Fatal("IR compile failure should mark fn cache as initialized")
	}
	if fn.irProg != irCompileFailed {
		t.Fatalf("IR compile failure should cache irCompileFailed sentinel, got %#v", fn.irProg)
	}
}

func TestNativeHelperEligibilityContract(t *testing.T) {
	pure := evalTestScript(t, `(fn [x y] (+ (* x x) y))`).(*Fn)
	pureProg := irGetFnProg(pure)
	nativeHelper, ok := runtimeExec.NativeHelper(pureProg)
	if pureProg == nil || !ok || !runtimeExec.NativeHelperChecked(pureProg) {
		t.Fatalf("pure numeric helper should compile native helper: %#v", pureProg)
	}
	if got := nativeHelper([]float64{3, 4}); got != 13 {
		t.Fatalf("native helper result = %f, want 13", got)
	}

	impure := evalTestScript(t, `(fn [x] [x])`).(*Fn)
	impureProg := irGetFnProg(impure)
	if impureProg == nil {
		t.Fatal("vector-building fn should still compile to boxed IR")
	}
	if runtimeExec.HasNativeHelper(impureProg) {
		t.Fatal("collection-building fn must not get a numeric native helper")
	}
	if !runtimeExec.NativeHelperChecked(impureProg) {
		t.Fatal("native helper eligibility should be checked and cached")
	}
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

// ---- runtime_execution_metadata_contract_test.go ----
func TestIRFunctionCacheUsesStableArityKeys(t *testing.T) {
	expr := compileBenchExpr(t, `(fn [x] (+ x 1))`)
	fn := Eval(expr, nil).(*Fn)
	first := irCompileFn(fn)
	if first == nil {
		t.Fatal("first irCompileFn returned nil")
	}
	second := irCompileFn(fn)
	if second == nil {
		t.Fatal("second irCompileFn returned nil")
	}
	if first != second {
		t.Fatalf("irCompileFn returned different programs for same fn: %p != %p", first, second)
	}
}

func TestIRFunctionCacheUsesStableVariadicKey(t *testing.T) {
	expr := compileBenchExpr(t, `(fn [& xs] (count xs))`)
	fn := Eval(expr, nil).(*Fn)
	first := irCompileFn(fn)
	if first == nil {
		t.Fatal("first variadic irCompileFn returned nil")
	}
	second := irCompileFn(fn)
	if second == nil {
		t.Fatal("second variadic irCompileFn returned nil")
	}
	if first != second {
		t.Fatalf("variadic irCompileFn returned different programs for same fn: %p != %p", first, second)
	}
}

func TestIREqSupportsStringsAndChars(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [f (fn [c]
                                  (if (= c "A") 1
                                  (if (= c "T") 2 3)))]
  (loop [i 0 acc 0]
    (if (= i 3)
      acc
      (recur (inc i) (+ acc (f (str (nth "ATA" i))))))))`), 4)
}

func TestIRMakeFnCapturesCurrentSlots(t *testing.T) {
	expr := compileBenchExpr(t, `(loop [x 10]
  (fn [y] (+ x y)))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("expected loop returning fn to compile to IR")
	}
	if len(prog.fnExprs) != 1 {
		t.Fatalf("expected root executable envelope to retain one FnExpr, got %d", len(prog.fnExprs))
	}
	model := prog.neutralModel()
	if model == nil || model.ConstantsLen != len(prog.constants) {
		t.Fatalf("neutral model mismatch: model=%#v constants=%d", model, len(prog.constants))
	}
	fnObj := irExec(prog, []coretypes.Object{coretypes.MakeInt(10)})
	fn, ok := fnObj.(*Fn)
	if !ok {
		t.Fatalf("irMakeFn result = %T, want *Fn", fnObj)
	}
	if fn.env == nil || len(fn.env.bindings) == 0 || !fn.env.bindings[0].Equals(coretypes.MakeInt(10)) {
		t.Fatalf("irMakeFn did not capture current slots: %#v", fn.env)
	}
	got := fn.Call([]coretypes.Object{coretypes.MakeInt(5)})
	if !got.Equals(coretypes.MakeInt(15)) {
		t.Fatalf("captured fn result = %s, want 15", got.ToString(false))
	}
}

func TestIRExecutionEnvelopeKeepsRuntimeMetadata(t *testing.T) {
	expr := compileBenchExpr(t, `(loop [i 0 s 0]
  (if (= i 5) s (recur (inc i) (+ s i))))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("expected loop to compile to IR")
	}
	model := prog.neutralModel()
	if model == nil {
		t.Fatal("expected neutral IR model")
	}
	if model.ConstantsLen != len(prog.constants) {
		t.Fatalf("model constants len = %d, envelope constants len = %d", model.ConstantsLen, len(prog.constants))
	}
	if model.NumSlots != prog.numSlots {
		t.Fatalf("model slots = %d, envelope slots = %d", model.NumSlots, prog.numSlots)
	}
	analysis := AnalyzeIRProgram(prog)
	if prog.escapeInfo == nil {
		t.Fatal("AnalyzeIRProgram should populate root execution escape metadata")
	}
	if model.Analysis == nil || model.Analysis.NumOps != analysis.NumOps {
		t.Fatalf("neutral model analysis not populated from execution analysis: model=%#v analysis=%#v", model.Analysis, analysis)
	}
	got := irExec(prog, []coretypes.Object{coretypes.MakeInt(0), coretypes.MakeInt(0)})
	if !got.Equals(coretypes.MakeInt(10)) {
		t.Fatalf("irExec result = %s, want 10", got.ToString(false))
	}
}

func TestIRFunctionEnvelopeKeepsCaptureMetadata(t *testing.T) {
	expr := compileBenchExpr(t, `(let [x 10] (fn [y] (+ x y)))`)
	fn := Eval(expr, nil).(*Fn)
	prog := irCompileFn(fn)
	if prog == nil {
		t.Fatal("expected captured fn to compile to IR")
	}
	if len(prog.captureKeys) == 0 || len(prog.captureSlotIdxs) == 0 {
		t.Fatalf("expected root envelope capture metadata: keys=%d idxs=%d", len(prog.captureKeys), len(prog.captureSlotIdxs))
	}
	model := prog.neutralModel()
	if model == nil {
		t.Fatal("expected neutral IR model")
	}
	if len(model.CaptureSlotIdxs) != len(prog.captureSlotIdxs) {
		t.Fatalf("model capture idxs = %d, envelope capture idxs = %d", len(model.CaptureSlotIdxs), len(prog.captureSlotIdxs))
	}
	got := fn.Call([]coretypes.Object{coretypes.MakeInt(5)})
	if !got.Equals(coretypes.MakeInt(15)) {
		t.Fatalf("captured fn result = %s, want 15", got.ToString(false))
	}
}

// ---- string_cursor_parse_test.go ----
const jsonCursorParserScript = `
(let [ws? (fn [c] (or (= c \space) (= c \newline) (= c \tab) (= c \return)))
      skip-ws (fn [cur]
        (loop [c cur]
          (if (cursor-done? c) c
            (if (ws? (cursor-char c)) (recur (cursor-next c)) c))))
      digit? (fn [c] (and (>= (int c) 48) (<= (int c) 57)))]
  (let [parse-string (fn [cur]
          (loop [c (cursor-next cur) buf ""]
            (let [ch (cursor-char c)]
              (if (= ch \") [buf (cursor-next c)]
                (if (= ch \\) (recur (cursor-next (cursor-next c)) (str buf (cursor-char (cursor-next c))))
                  (recur (cursor-next c) (str buf ch)))))))
        parse-number (fn [cur]
          (let [ch (cursor-char cur)
                neg (= ch \-)
                c (if neg (cursor-next cur) cur)]
            (loop [c2 c n 0]
              (if (cursor-done? c2) [(if neg (- 0 n) n) c2]
                (let [ch2 (cursor-char c2)]
                  (if (digit? ch2)
                    (recur (cursor-next c2) (+ (* n 10) (- (int ch2) 48)))
                    [(if neg (- 0 n) n) c2]))))))]
    (let [pv-ref (atom nil)
          parse-array (fn [cur]
            (let [pv @pv-ref
                  c2 (skip-ws (cursor-next cur))]
              (if (= (cursor-char c2) \])
                [[] (cursor-next c2)]
                (loop [c3 c2 arr []]
                  (let [[val nc] (pv c3)
                        nc2 (skip-ws nc)]
                    (if (= (cursor-char nc2) \])
                      [(conj arr val) (cursor-next nc2)]
                      (recur (skip-ws (cursor-next nc2)) (conj arr val))))))))
          parse-object (fn [cur]
            (let [pv @pv-ref
                  c2 (skip-ws (cursor-next cur))]
              (if (= (cursor-char c2) \})
                [{} (cursor-next c2)]
                (loop [c3 c2 m {}]
                  (let [[key nc] (parse-string c3)
                        nc2 (skip-ws nc)
                        nc3 (skip-ws (cursor-next nc2))
                        [val nc4] (pv nc3)
                        nc5 (skip-ws nc4)]
                    (if (= (cursor-char nc5) \})
                      [(assoc m key val) (cursor-next nc5)]
                      (recur (skip-ws (cursor-next nc5)) (assoc m key val))))))))]
      (let [parse-value (fn [cur]
              (let [c2 (skip-ws cur)
                    ch (cursor-char c2)]
                (if (= ch \") (parse-string c2)
                  (if (= ch \{) (parse-object c2)
                    (if (= ch \[) (parse-array c2)
                      (if (= ch \t) [true (cursor-next (cursor-next (cursor-next (cursor-next c2))))]
                        (if (= ch \f) [false (cursor-next (cursor-next (cursor-next (cursor-next (cursor-next c2)))))]
                          (if (= ch \n) [nil (cursor-next (cursor-next (cursor-next (cursor-next c2))))]
                            (parse-number c2)))))))))
            _ (reset! pv-ref parse-value)]
        (fn [s] (first (parse-value (string-cursor s))))))))
`

func getCursorJSONParser(tb testing.TB) coretypes.Callable {
	initStringCursorProcs()
	return Eval(compileBenchExpr(tb, jsonCursorParserScript), nil).(coretypes.Callable)
}

func TestCursorJSONCorrectness(t *testing.T) {
	parse := getCursorJSONParser(t)
	result := parse.Call([]coretypes.Object{coretypes.String{S: jsonSmall}})
	if result == nil || result == NIL {
		t.Fatal("returned nil")
	}
	t.Logf("result type: %T", result)
}

// ---- string_ir_fastpath_test.go ----
func TestStringNthFastUnicodeCorrectness(t *testing.T) {
	got := stringNthFast("abcdef", 3)
	if ch, ok := got.(coretypes.Char); !ok || ch.Ch != 'd' {
		t.Fatalf("expected d, got %T %s", got, got.ToString(false))
	}
	got = stringNthFast("éclair", 1)
	if ch, ok := got.(coretypes.Char); !ok || ch.Ch != 'c' {
		t.Fatalf("expected c, got %T %s", got, got.ToString(false))
	}
}

func TestIRNthStringFastPath(t *testing.T) {
	requireString(t, evalTestScript(t, `(loop [i 0 s ""]
  (if (= i 3)
    s
    (recur (inc i) (str s (nth "abcdef" i)))))`), "abc")
}

func TestCharToStringFast(t *testing.T) {
	if got := charToStringFast('A'); got != "A" {
		t.Fatalf("expected A, got %q", got)
	}
	if got := charToStringFast('é'); got != "é" {
		t.Fatalf("expected é, got %q", got)
	}
	if got := charToStringObjectFast('A'); got.(coretypes.String).S != "A" {
		t.Fatalf("expected cached A object, got %T %s", got, got.ToString(false))
	}
	if got := charToStringObjectFast('é'); got.(coretypes.String).S != "é" {
		t.Fatalf("expected unicode string object, got %T %s", got, got.ToString(false))
	}
}

func TestIRNthStringASCIIOpcode(t *testing.T) {
	expr := compileTestExpr(t, `(let [dna "ACGT"]
  (loop [i 0 acc ""]
    (if (= i 4)
      acc
      (recur (inc i) (str acc (nth dna i))))))`)
	letExpr := expr.(*LetExpr)
	loop := letExpr.body[0].(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Fatal("expected IR")
	}
	found := false
	for pc := 0; pc < len(prog.code); pc++ {
		if prog.code[pc] == irNthStringASCII {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected irNthStringASCII opcode")
	}
	requireString(t, Eval(expr, nil), "ACGT")
}

// ---- test_helpers_test.go ----
func compileBenchExpr(tb testing.TB, script string) Expr {
	tb.Helper()
	reader := NewReader(strings.NewReader(script), "<bench>")
	obj, err := TryRead(reader)
	if err != nil {
		tb.Fatalf("read script: %v", err)
	}
	if _, err := TryRead(reader); err != io.EOF {
		tb.Fatalf("benchmark script must contain exactly one form")
	}
	expr, err := TryParse(obj, &ParseContext{GlobalEnv: GLOBAL_ENV})
	if err != nil {
		tb.Fatalf("parse script: %v", err)
	}
	return expr
}

var clbgInitOnce sync.Once

func clbgInit() {
	clbgInitOnce.Do(func() {
		sqrtProc := Proc{Fn: func(args []coretypes.Object) coretypes.Object {
			x := coretypes.EnsureArgIsNumber(args, 0).Double().D
			return coretypes.Double{D: math.Sqrt(x)}
		}, Name: "procSqrt"}
		vr := GLOBAL_ENV.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sqrt"))
		vr.Value = sqrtProc
		ns := GLOBAL_ENV.CurrentNamespace()
		uv := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sqrt"))
		uv.Value = sqrtProc
	})
}

const nbodyScript = `
(let [pi 3.141592653589793
      solar-mass (* 4.0 (* pi pi))
      days-per-year 365.24
      ;; bodies: [x y z vx vy vz mass] as flat vector
      ;; Jupiter, Saturn, Uranus, Neptune + Sun
      initial-bodies
      [;; Sun
       0.0 0.0 0.0 0.0 0.0 0.0 solar-mass
       ;; Jupiter
       4.84143144246472090 -1.16032004402742839 -0.103622044471123109
       (* 0.00166007664274403694 days-per-year)
       (* 0.00769901118419740425 days-per-year)
       (* -0.0000690460016972063023 days-per-year)
       (* 0.000954791938424326609 solar-mass)
       ;; Saturn
       8.34336671824457987 4.12479856412430479 -0.403523417114321381
       (* -0.00276742510726862411 days-per-year)
       (* 0.00499852801234917238 days-per-year)
       (* 0.0000230417297573763929 days-per-year)
       (* 0.000285885980666130812 solar-mass)
       ;; Uranus
       12.8943695621391310 -15.1111514016986312 -0.223307578892655734
       (* 0.00296460137564761618 days-per-year)
       (* 0.00237847173959480950 days-per-year)
       (* -0.0000296589568540237556 days-per-year)
       (* 0.0000436624404335156298 solar-mass)
       ;; Neptune
       15.3796971148509165 -25.9193146099879641 0.179258772950371181
       (* 0.00268067772490389322 days-per-year)
       (* 0.00162824170038242295 days-per-year)
       (* -0.0000951592254519715870 days-per-year)
       (* 0.0000515138902046611451 solar-mass)]
      n-bodies 5
      body-size 7
      get-x (fn [bs i] (nth bs (* i body-size)))
      get-y (fn [bs i] (nth bs (+ (* i body-size) 1)))
      get-z (fn [bs i] (nth bs (+ (* i body-size) 2)))
      get-vx (fn [bs i] (nth bs (+ (* i body-size) 3)))
      get-vy (fn [bs i] (nth bs (+ (* i body-size) 4)))
      get-vz (fn [bs i] (nth bs (+ (* i body-size) 5)))
      get-m (fn [bs i] (nth bs (+ (* i body-size) 6)))
      set-at (fn [bs idx val] (assoc bs idx val))
      advance
      (fn [bs dt]
        (loop [i 0 b bs]
          (if (= i n-bodies)
            b
            (let [ix (get-x b i) iy (get-y b i) iz (get-z b i)
                  im (get-m b i)
                  b2 (loop [j (+ i 1) bj b vxi (get-vx b i) vyi (get-vy b i) vzi (get-vz b i)]
                       (if (= j n-bodies)
                         (let [base (* i body-size)]
                           (set-at (set-at (set-at bj (+ base 3) vxi) (+ base 4) vyi) (+ base 5) vzi))
                         (let [jx (get-x bj j) jy (get-y bj j) jz (get-z bj j)
                               jm (get-m bj j)
                               dx (- ix jx) dy (- iy jy) dz (- iz jz)
                               dist2 (+ (* dx dx) (+ (* dy dy) (* dz dz)))
                               dist (sqrt dist2)
                               mag (/ dt (* dist2 dist))
                               vxi2 (- vxi (* dx (* jm mag)))
                               vyi2 (- vyi (* dy (* jm mag)))
                               vzi2 (- vzi (* dz (* jm mag)))
                               jvx (+ (get-vx bj j) (* dx (* im mag)))
                               jvy (+ (get-vy bj j) (* dy (* im mag)))
                               jvz (+ (get-vz bj j) (* dz (* im mag)))
                               jbase (* j body-size)
                               bj2 (set-at (set-at (set-at bj (+ jbase 3) jvx) (+ jbase 4) jvy) (+ jbase 5) jvz)]
                           (recur (+ j 1) bj2 vxi2 vyi2 vzi2))))]
              (let [base (* i body-size)
                    vx (get-vx b2 i) vy (get-vy b2 i) vz (get-vz b2 i)
                    b3 (set-at (set-at (set-at b2 base (+ ix (* dt vx)))
                                       (+ base 1) (+ iy (* dt vy)))
                               (+ base 2) (+ iz (* dt vz)))]
                (recur (+ i 1) b3))))))
      energy
      (fn [bs]
        (loop [i 0 e 0.0]
          (if (= i n-bodies)
            e
            (let [vx (get-vx bs i) vy (get-vy bs i) vz (get-vz bs i)
                  m (get-m bs i)
                  ke (* 0.5 (* m (+ (* vx vx) (+ (* vy vy) (* vz vz)))))
                  pe (loop [j (+ i 1) pe2 0.0]
                       (if (= j n-bodies)
                         pe2
                         (let [dx (- (get-x bs i) (get-x bs j))
                               dy (- (get-y bs i) (get-y bs j))
                               dz (- (get-z bs i) (get-z bs j))
                               dist (sqrt (+ (* dx dx) (+ (* dy dy) (* dz dz))))]
                           (recur (+ j 1) (- pe2 (/ (* m (get-m bs j)) dist))))))]
              (recur (+ i 1) (+ e (- ke pe)))))))]
  (loop [step 0 bodies initial-bodies]
    (if (= step 100)
      (energy bodies)
      (recur (+ step 1) (advance bodies 0.01)))))
`

// spectral-norm: compute spectral norm of an infinite matrix, N=50.
// Scaled down from CLBG's N=5500 for benchmark harness practicality.
const spectralNormScript = `
(let [n 50
      A (fn [i j] (/ 1.0 (+ (/ (* (+ i j) (+ (+ i j) 1)) 2) (+ i 1))))
      mul-Av (fn [v n]
        (loop [i 0 result []]
          (if (= i n)
            result
            (let [s (loop [j 0 s 0.0]
                      (if (= j n)
                        s
                        (recur (+ j 1) (+ s (* (A i j) (nth v j))))))]
              (recur (+ i 1) (conj result s))))))
      mul-Atv (fn [v n]
        (loop [i 0 result []]
          (if (= i n)
            result
            (let [s (loop [j 0 s 0.0]
                      (if (= j n)
                        s
                        (recur (+ j 1) (+ s (* (A j i) (nth v j))))))]
              (recur (+ i 1) (conj result s))))))
      mul-AtAv (fn [v n] (mul-Atv (mul-Av v n) n))
      initial-u (loop [i 0 v []]
                  (if (= i n) v (recur (+ i 1) (conj v 1.0))))]
  (loop [iter 0 u initial-u v []]
    (if (= iter 10)
      (let [vBv (loop [i 0 s 0.0]
                  (if (= i n) s (recur (+ i 1) (+ s (* (nth u i) (nth v i))))))
            vv (loop [i 0 s 0.0]
                 (if (= i n) s (recur (+ i 1) (+ s (* (nth v i) (nth v i))))))]
        (sqrt (/ vBv vv)))
      (let [v2 (mul-AtAv u n)
            u2 (mul-AtAv v2 n)]
        (recur (+ iter 1) u2 v2)))))
`

// binary-trees: allocate and check binary trees, depth=14.
// Scaled down from CLBG's depth=21 for benchmark harness practicality.
const binaryTreesScript = `
(letfn [(make-tree [depth]
          (if (= depth 0)
            [:leaf]
            (let [d (- depth 1)]
              [:node (make-tree d) (make-tree d)])))
        (check-tree [tree]
          (if (= (first tree) :leaf)
            1
            (+ 1 (+ (check-tree (nth tree 1)) (check-tree (nth tree 2))))))]
  (loop [d 4 total 0]
    (if (= d 15)
      total
      (let [iterations (loop [i 0 n 1] (if (= i (- 14 d)) n (recur (+ i 1) (* n 2))))
            check (loop [i 0 c 0]
                    (if (= i iterations)
                      c
                      (recur (+ i 1) (+ c (check-tree (make-tree d))))))]
        (recur (+ d 1) (+ total check))))))
`

const binaryTreesParallelScript = `
(letfn [(make-tree [depth]
          (if (= depth 0)
            [:leaf]
            (let [d (- depth 1)]
              [:node (make-tree d) (make-tree d)])))
        (check-tree [tree]
          (if (= (first tree) :leaf)
            1
            (+ 1 (+ (check-tree (nth tree 1)) (check-tree (nth tree 2))))))
        (depth-check [d]
          (let [iterations (loop [i 0 n 1] (if (= i (- 14 d)) n (recur (+ i 1) (* n 2))))]
            (loop [i 0 c 0]
              (if (= i iterations)
                c
                (recur (+ i 1) (+ c (check-tree (make-tree d))))))))]
  (reduce + 0 (pmap depth-check (range 4 15))))
`

const jsonSmall = `{"name":"John","age":30,"city":"New York","active":true,"scores":[95,87,92]}`
const jsonMedium = `[{"id":1,"name":"Alice","email":"alice@test.com","tags":["admin","user"],"score":95},{"id":2,"name":"Bob","email":"bob@test.com","tags":["user"],"score":87},{"id":3,"name":"Charlie","email":"charlie@test.com","tags":["user","mod"],"score":92},{"id":4,"name":"Dave","email":"dave@test.com","tags":[],"score":78},{"id":5,"name":"Eve","email":"eve@test.com","tags":["admin"],"score":99}]`

// ---- transient_test.go ----
func TestTransientVector(t *testing.T) {
	v := &corecollections.ArrayVector{Arr: []coretypes.Object{coretypes.Int{I: 1}, coretypes.Int{I: 2}, coretypes.Int{I: 3}}}
	tv := coretypes.ToTransient(v.Arr)
	tv.AssocInPlace(coretypes.Int{I: 1}, coretypes.Int{I: 99})
	tv.ConjInPlace(coretypes.Int{I: 4})
	if tv.Count() != 4 {
		t.Fatalf("expected count 4, got %d", tv.Count())
	}
	if tv.Nth(1).(coretypes.Int).I != 99 {
		t.Fatalf("expected 99 at index 1")
	}
	pv := coretypes.EnsureObjectIsCountedIndexed(tv.ToPersistent(), "")
	if pv.Count() != 4 {
		t.Fatalf("persistent count wrong")
	}
}

func TestTransientMapZeroValueAssoc(t *testing.T) {
	tm := &coretypes.TransientMap{}
	tm.AssocInPlace(coretypes.MakeKeyword(STRINGS.Intern, "a"), coretypes.Int{I: 1})
	if ok, got := tm.Get(coretypes.MakeKeyword(STRINGS.Intern, "a")); !ok || !got.Equals(coretypes.Int{I: 1}) {
		t.Fatalf("zero-value transient map lookup = %v %v", ok, got)
	}
}

func TestTransientMap(t *testing.T) {
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "a"), coretypes.Int{I: 1})
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "b"), coretypes.Int{I: 2})
	tm := coretypes.MapToTransient(m)
	tm.AssocInPlace(coretypes.MakeKeyword(STRINGS.Intern, "c"), coretypes.Int{I: 3})
	tm.AssocInPlace(coretypes.MakeKeyword(STRINGS.Intern, "a"), coretypes.Int{I: 99})
	if tm.Count() != 3 {
		t.Fatalf("expected 3, got %d", tm.Count())
	}
	ok, v := tm.Get(coretypes.MakeKeyword(STRINGS.Intern, "a"))
	if !ok || v.(coretypes.Int).I != 99 {
		t.Fatalf("expected 99 for :a")
	}
	pm := tm.ToPersistent()
	if pm == nil {
		t.Fatal("persistent returned nil")
	}
}

func TestTransientMapStringKeys(t *testing.T) {
	tm := coretypes.MapToTransient(nil)
	tm.AssocInPlace(coretypes.String{S: "alpha"}, coretypes.Int{I: 1})
	tm.AssocInPlace(coretypes.String{S: "beta"}, coretypes.Int{I: 2})
	tm.AssocInPlace(coretypes.String{S: "alpha"}, coretypes.Int{I: 3})
	if tm.Count() != 2 {
		t.Fatalf("expected 2, got %d", tm.Count())
	}
	ok, v := tm.Get(coretypes.String{S: "alpha"})
	if !ok || v.(coretypes.Int).I != 3 {
		t.Fatalf("expected 3 for alpha")
	}
	pm := tm.ToPersistent().(coretypes.Map)
	ok, v = pm.Get(coretypes.String{S: "beta"})
	if !ok || v.(coretypes.Int).I != 2 {
		t.Fatalf("expected persistent beta=2")
	}
}

func TestTransientVectorProcs(t *testing.T) {
	vec := corecollections.NewArrayVectorFrom(coretypes.Int{I: 1}, coretypes.Int{I: 2})
	tv, ok := procTransient([]coretypes.Object{vec}).(*coretypes.TransientVector)
	if !ok {
		t.Fatalf("transient vector proc returned %T", tv)
	}
	if got := procIsTransient([]coretypes.Object{tv}); !got.Equals(coretypes.Boolean{B: true}) {
		t.Fatalf("transient? returned %s", got.ToString(false))
	}
	if procAssocBang([]coretypes.Object{tv, coretypes.Int{I: 1}, coretypes.Int{I: 20}}) != tv {
		t.Fatal("assoc! should return the same transient vector")
	}
	assertPanics(t, "assoc! transient vector key type", func() {
		procAssocBang([]coretypes.Object{tv, coretypes.MakeKeyword(STRINGS.Intern, "bad"), coretypes.Int{I: 0}})
	})
	if procConjBang([]coretypes.Object{tv, coretypes.Int{I: 3}}) != tv {
		t.Fatal("conj! should return the same transient vector")
	}
	if tv.Count() != 3 || !tv.At(1).Equals(coretypes.Int{I: 20}) || !tv.At(2).Equals(coretypes.Int{I: 3}) {
		t.Fatalf("unexpected transient vector state: count=%d", tv.Count())
	}
	if procPopBang([]coretypes.Object{tv}) != tv {
		t.Fatal("pop! should return the same transient vector")
	}
	persisted := procPersistentBang([]coretypes.Object{tv}).(*corecollections.ArrayVector)
	if persisted.Count() != 2 || !persisted.At(1).Equals(coretypes.Int{I: 20}) {
		t.Fatalf("unexpected persistent vector: %s", persisted.ToString(false))
	}
}

func TestTransientMapProcs(t *testing.T) {
	tm, ok := procTransient([]coretypes.Object{corecollections.EmptyArrayMap()}).(*coretypes.TransientMap)
	if !ok {
		t.Fatalf("transient map proc returned %T", tm)
	}
	if got := procIsTransient([]coretypes.Object{tm}); !got.Equals(coretypes.Boolean{B: true}) {
		t.Fatalf("transient? returned %s", got.ToString(false))
	}
	if procAssocBang([]coretypes.Object{tm, coretypes.MakeKeyword(STRINGS.Intern, "a"), coretypes.Int{I: 1}}) != tm {
		t.Fatal("assoc! should return the same transient map")
	}
	if procConjBang([]coretypes.Object{tm, coretypes.String{S: "b"}, coretypes.Int{I: 2}}) != tm {
		t.Fatal("conj! should return the same transient map")
	}
	persisted := procPersistentBang([]coretypes.Object{tm}).(coretypes.Map)
	if persisted.Count() != 2 {
		t.Fatalf("persistent map count = %d", persisted.Count())
	}
	if ok, got := persisted.Get(coretypes.String{S: "b"}); !ok || !got.Equals(coretypes.Int{I: 2}) {
		t.Fatalf("missing persisted string key: %v %v", ok, got)
	}
}

func TestIRTransientStringBuilder(t *testing.T) {
	t.Setenv("JOKER_IR_STRING_BUILDER", "force")
	requireString(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      s
      (recur (inc i) (str s (nth dna i))))))`), "ACGT")
}

func TestIRTransientStringPrependAuto(t *testing.T) {
	requireString(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      s
      (recur (inc i) (str (nth dna i) s)))))`), "TGCA")
}

// ---- typed_helpers_test.go ----
func TestIRValueToString(t *testing.T) {
	cases := []struct {
		v irValue
		w string
	}{
		{irValue{tag: irValInt, i: 42}, "42"},
		{irMakeChar('A'), "A"},
		{irMakeString("abc", 3, true), "abc"},
		{irMakeBool(true), "true"},
		{irValue{tag: irValNil}, ""},
	}
	for _, c := range cases {
		if got := irValueToString(c.v); got != c.w {
			t.Fatalf("expected %q, got %q", c.w, got)
		}
	}
}

func TestIRTypedUnicodeCount(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 s "é"]
  (if (= i 2)
    (count s)
    (recur (inc i) (str s "é"))))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	requireInt(t, irExecTyped(prog, []coretypes.Object{coretypes.Int{I: 0}, coretypes.String{S: "é"}}), 3)
}

func TestIRTypedCountObjectVector(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [xs ["a" "b" "c"]]
  (loop [i 0 s ""]
    (if (= i (count xs))
      (count s)
      (recur (inc i) (str s "x")))))`), 3)
}

func TestIRTypedEvalGate(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED", "1")
	requireInt(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      (count s)
      (recur (inc i) (str s (nth dna i))))))`), 4)
}

func TestIRTypedMapRejectsNonStringKeysAndFallsBack(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [ks [:k0 :k1]]
  (loop [i 0 m {}]
    (if (= i 4)
      (+ (get m :k0 0) (get m :k1 0))
      (let [k (nth ks (rem i 2))]
        (recur (inc i) (assoc m k (inc (get m k 0))))))))`), 4)
}

func TestIRTypedNestedStringLoop(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 total 0]
    (if (= i 3)
      total
      (let [k (loop [j 0 s ""]
                (if (= j 2)
                  s
                  (recur (inc j) (str s (nth dna (+ i j))))))]
        (recur (inc i) (+ total (count k)))))))`), 6)
}

func TestIRTypedIntVector(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED_VEC", "1")
	requireInt(t, evalTestScript(t, `(loop [i 0 v []]
  (if (= i 5)
    (+ (nth v 0) (nth v 4))
    (recur (inc i) (conj v i))))`), 4)
}

func TestIRTypedGenericStringNth(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 s "ACGT" acc ""]
  (if (= i 4)
    acc
    (recur (inc i) s (str acc (nth s i)))))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	got := irExecTyped(prog, []coretypes.Object{coretypes.Int{I: 0}, coretypes.String{S: "ACGT"}, coretypes.String{S: ""}})
	requireString(t, got, "ACGT")
}

func TestIRTypedStringIntMap(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED_MAP", "1")
	requireInt(t, evalTestScript(t, `(loop [i 0 m {}]
  (if (= i 4)
    (get m "aa" 0)
    (recur (inc i) (assoc m "aa" (inc (get m "aa" 0))))))`), 4)
	requireBool(t, evalTestScript(t, `(nil? (loop [i 0 m {}]
  (if (= i 1)
    (get m "missing")
    (recur (inc i) m))))`), true)
}

func TestIRTypedVectorNthForStringMap(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED_MAP", "1")
	requireInt(t, evalTestScript(t, `(let [ks ["aa" "bb"]]
  (loop [i 0 m {}]
    (if (= i 4)
      (get m "aa" 0)
      (let [k (nth ks (rem i 2))]
        (recur (inc i) (assoc m k (inc (get m k 0))))))))`), 2)
}

func TestIRTypedStringLoop(t *testing.T) {
	expr := compileTestExpr(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      (count s)
      (recur (inc i) (str s (nth dna i))))))`)
	letExpr := expr.(*LetExpr)
	prog := irCompile(letExpr.body[0].(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	got := irExecTyped(prog, []coretypes.Object{coretypes.Int{I: 0}, coretypes.String{S: ""}})
	requireInt(t, got, 4)
}

func TestIRTypedStringBuilderSlot(t *testing.T) {
	expr := compileTestExpr(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      s
      (recur (inc i) (str s (nth dna i))))))`)
	letExpr := expr.(*LetExpr)
	prog := irCompile(letExpr.body[0].(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	got := irExecTyped(prog, []coretypes.Object{coretypes.Int{I: 0}, coretypes.String{S: ""}})
	requireString(t, got, "ACGT")
}

// ---- wasm_compile_test.go ----
func TestWasmArithmeticLoopCorrectness(t *testing.T) {
	expr := compileBenchExpr(t, `(loop [i 0 s 0]
  (if (= i 100) s (recur (inc i) (+ s (rem (* i 7) 11)))))`)
	// Get expected result from IR
	expected := Eval(expr, nil)

	// Try WASM compilation
	loop := expr.(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Skip("IR compilation failed")
	}
	model := prog.neutralModel()
	if model == nil || !corewasm.Eligible(model.Code) {
		t.Skip("IR not WASM-eligible")
	}
	wp := wasmCompile(prog)
	if wp == nil {
		t.Skip("WASM compilation failed")
	}
	result := wasmExec(wp, []coretypes.Object{coretypes.Int{I: 0}, coretypes.Int{I: 0}})
	if result == nil {
		t.Fatal("WASM execution returned nil")
	}
	if !result.Equals(expected) {
		t.Fatalf("WASM result %s != IR result %s", result.ToString(false), expected.ToString(false))
	}
}

func TestWasmSimpleLoop(t *testing.T) {
	expr := compileBenchExpr(t, `(loop [i 0 s 0]
  (if (= i 10) s (recur (+ i 1) (+ s i))))`)
	expected := Eval(expr, nil)

	loop := expr.(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Skip("IR failed")
	}
	wp := wasmCompile(prog)
	if wp == nil {
		t.Skip("WASM failed")
	}
	result := wasmExec(wp, []coretypes.Object{coretypes.Int{I: 0}, coretypes.Int{I: 0}})
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.Equals(expected) {
		t.Fatalf("got %s, want %s", result.ToString(false), expected.ToString(false))
	}
}

func TestWasmFloatLoop(t *testing.T) {
	expr := compileBenchExpr(t, `(loop [x 0.0 i 0]
  (if (= i 100) x (recur (+ x (* 0.5 0.5)) (inc i))))`)
	expected := Eval(expr, nil)
	loop := expr.(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Skip("IR failed")
	}
	model := prog.neutralModel()
	if model == nil || !corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0) {
		t.Fatal("float loop should be detected as using float operations/constants")
	}
	if prog.model == nil {
		t.Fatal("compiled IR program should populate neutral model")
	}
	analysis := AnalyzeIRProgram(prog)
	if prog.model.Analysis == nil || !prog.model.Analysis.UsesFloat || !analysis.UsesFloat {
		t.Fatalf("neutral model analysis should preserve float usage: model=%#v analysis=%#v", prog.model.Analysis, analysis)
	}
	t.Logf("float: %v, eligible: %v", corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0), corewasm.Eligible(model.Code))
	wp := wasmCompile(prog)
	if wp == nil {
		t.Skip("WASM failed")
	}
	result := wasmExec(wp, []coretypes.Object{coretypes.Double{D: 0.0}, coretypes.Int{I: 0}})
	if result == nil {
		t.Fatal("nil")
	}
	t.Logf("WASM=%s IR=%s", result.ToString(false), expected.ToString(false))
	// Allow small FP difference
	if result.ToString(false) != expected.ToString(false) {
		t.Fatalf("mismatch: WASM=%s IR=%s", result.ToString(false), expected.ToString(false))
	}
}

func TestWasmArrayRejectsInvalidSizes(t *testing.T) {
	if arr := corewasm.MakeF64ArrayWithRuntime(getWasmRT, -1, TYPE.ArrayVector); arr != nil {
		t.Fatalf("MakeF64Array(-1) = %#v, want nil", arr)
	}
	if arr := corewasm.MakeI64ArrayWithRuntime(getWasmRT, -1, TYPE.ArrayVector); arr != nil {
		t.Fatalf("MakeI64Array(-1) = %#v, want nil", arr)
	}
}

func TestWasmArrayF64(t *testing.T) {
	arr := corewasm.MakeF64ArrayWithRuntime(getWasmRT, 10, TYPE.ArrayVector)
	if arr == nil {
		t.Skip("WASM array allocation failed")
	}
	arr.SetF64(0, 3.14)
	arr.SetF64(5, 2.71)
	if arr.GetF64(0) != 3.14 {
		t.Fatalf("expected 3.14, got %f", arr.GetF64(0))
	}
	if arr.GetF64(5) != 2.71 {
		t.Fatalf("expected 2.71, got %f", arr.GetF64(5))
	}
	if arr.GetF64(3) != 0 {
		t.Fatalf("expected 0, got %f", arr.GetF64(3))
	}
	if arr.Length() != 10 {
		t.Fatalf("expected length 10, got %d", arr.Length())
	}
}

func TestWasmRawIntObjectPromotesOutsideNativeRange(t *testing.T) {
	got := corewasm.RawIntObject(uint64(math.MaxInt64))
	if math.MaxInt64 > int64(coretypes.MaxInt) {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("wasm raw int object type = %s, want coretypes.BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("wasm raw int object type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestWasmRawIntRejectsOutOfRangeIndex(t *testing.T) {
	if _, ok := corewasm.RawInt(uint64(math.MaxInt64)); ok && math.MaxInt64 > int64(coretypes.MaxInt) {
		t.Fatal("wasmRawInt should reject values outside native int range")
	}
}

func TestWasmExecRawIntegerResultUsesNativeRange(t *testing.T) {
	got := corewasm.RawIntObject(uint64(math.MaxInt64))
	if math.MaxInt64 > int64(coretypes.MaxInt) && got.GetType() != TYPE.BigInt {
		t.Fatalf("raw wasm result type = %s, want coretypes.BigInt", got.GetType().ToString(false))
	}
}

func TestWasmMemNthSimple(t *testing.T) {
	code := `(let [v [10.0 20.0 30.0]]
	  (loop [j 0 s 0.0]
	    (if (= j 3) s
	      (recur (+ j 1) (+ s (nth v j))))))`
	expr := compileTestExpr(t, code)
	letExpr := expr.(*LetExpr)
	loop := letExpr.body[0].(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Fatal("not compiled")
	}
	t.Logf("eligible=%v", wasmMemNthEligible(prog, nil))
	r := Eval(expr, nil)
	t.Logf("eval result=%s", r.ToString(false))
	requireDouble(t, r, 60.0)
}

func TestWasmMemNthWithHelper(t *testing.T) {
	clbgInit()
	code := `(let [A (fn [i j] (/ 1.0 (+ (/ (* (+ i j) (+ (+ i j) 1)) 2) (+ i 1))))
	              v [1.0 1.0 1.0 1.0 1.0]]
	  (loop [j 0 s 0.0]
	    (if (= j 5) s
	      (recur (+ j 1) (+ s (* (A 0 j) (nth v j)))))))`
	r := evalTestScript(t, code)
	t.Logf("result=%s", r.ToString(false))
}

// ---- wasm_helper_backend_test.go ----
func TestWasmOneHelperModule(t *testing.T) {
	// Use a helper with a string op so auto-inlining won't absorb it
	// (auto inlines text helpers but not ones with both text and non-text mixed patterns)
	t.Setenv("JOKER_IR_INLINE", "off")
	expr := compileTestExpr(t, `(let [f (fn [x] (+ (* x x) 1))]
  (loop [i 0 acc 0]
    (if (= i 5)
      acc
      (recur (inc i) (+ acc (f i))))))`)
	letExpr := expr.(*LetExpr)
	fnObj := Eval(letExpr.values[0], nil)
	loop := letExpr.body[0].(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Fatal("expected loop IR")
	}
	slots := make([]coretypes.Object, prog.numSlots)
	slots[0] = coretypes.Int{I: 0}
	slots[1] = coretypes.Int{I: 0}
	slots[2] = fnObj
	wp := wasmGetCachedWithOneHelper(prog, slots)
	if wp == nil {
		t.Fatal("expected one-helper WASM module")
	}
	requireInt(t, wasmExec(wp, slots), 35)
}

func TestWasmOneHelperFloatRequiresForce(t *testing.T) {
	expr := compileTestExpr(t, `(let [f (fn [x] (* x 2.0))]
  (loop [i 0 acc 0.0]
    (if (= i 2)
      acc
      (recur (inc i) (+ acc (f 1.5))))))`)
	letExpr := expr.(*LetExpr)
	fnObj := Eval(letExpr.values[0], nil)
	loop := letExpr.body[0].(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Fatal("expected loop IR")
	}
	slots := make([]coretypes.Object, prog.numSlots)
	slots[0] = coretypes.Int{I: 0}
	slots[1] = coretypes.Double{D: 0}
	slots[2] = fnObj
	if wp := wasmGetCachedWithOneHelper(prog, slots); wp != nil {
		t.Fatal("float helper should be gated off in auto mode")
	}
}
