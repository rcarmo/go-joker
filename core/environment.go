package core

import (
	"fmt"
	corert "github.com/rcarmo/go-joker/core/runtime"
	"io"
	"os"
	"sync"

	coretypes "github.com/rcarmo/go-joker/core/types"

	"github.com/rcarmo/go-joker/core/osutil"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

var (
	Stdin          io.Reader = os.Stdin
	Stdout         io.Writer = os.Stdout
	Stderr         io.Writer = os.Stderr
	VerbosityLevel           = 0
)

type (
	Env struct {
		Namespaces    map[*string]*Namespace
		CoreNamespace *Namespace
		stdout        *Var
		stdin         *Var
		stderr        *Var
		printReadably *Var
		file          *Var
		MainFile      *Var
		args          *Var
		classPath     *Var
		ns            *Var
		NS_VAR        *Var
		IN_NS_VAR     *Var
		version       *Var
		libs          *Var
		Features      coretypes.Set
	}
)

func versionMap() coretypes.Map {
	res := corecollections.EmptyArrayMap()
	major, minor, incremental := corestr.ParseVersionTriplet(VERSION)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "major"), coretypes.Int{I: int(major)})
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "minor"), coretypes.Int{I: int(minor)})
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "incremental"), coretypes.Int{I: int(incremental)})
	return res
}

func (env *Env) SetEnvArgs(newArgs []string) {
	args := corecollections.EmptyArrayVector()
	for _, arg := range newArgs {
		args.Append(coretypes.MakeString(arg))
	}
	if args.Count() > 0 {
		env.args.Value = args.Seq()
	} else {
		env.args.Value = NIL
	}
}

func (env *Env) SetClassPath(cp string) {
	cpVec := corecollections.EmptyArrayVector()
	for _, cpelem := range osutil.ClassPathElements(cp) {
		cpVec.Append(coretypes.MakeString(cpelem))
	}
	env.classPath.Value = cpVec
}

func (env *Env) InitEnv(stdin io.Reader, stdout, stderr io.Writer, args []string) {
	env.stdin.Value = MakeBufferedReader(stdin)
	env.stdout.Value = MakeIOWriter(stdout)
	env.stderr.Value = MakeIOWriter(stderr)
	if vr := env.CoreNamespace.Resolve("constantly"); vr != nil {
		vr.Value = Proc{Name: "procConstantly", Fn: func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			x := args[0]
			return Proc{Name: "procConstantlyValue", Fn: func(_ []coretypes.Object) coretypes.Object { return x }}
		}}
	}
	env.SetEnvArgs(args)
}

func (env *Env) SetStdIO(stdin, stdout, stderr coretypes.Object) {
	env.stdin.Value = stdin
	env.stdout.Value = stdout
	env.stderr.Value = stderr
}

func (env *Env) StdIO() (stdin, stdout, stderr coretypes.Object) {
	return env.stdin.Value, env.stdout.Value, env.stderr.Value
}

func (env *Env) SetMainFilename(filename string) {
	env.MainFile.Value = coretypes.MakeString(filename)
}

func (env *Env) SetFilename(obj coretypes.Object) {
	env.file.Value = obj
}

func (env *Env) IsStdIn(obj coretypes.Object) bool {
	return env.stdin.Value == obj
}

func (env *Env) CurrentNamespace() *Namespace {
	return EnsureObjectIsNamespace(env.ns.Value, "")
}

func (env *Env) SetCurrentNamespace(ns *Namespace) {
	env.ns.Value = ns
}

func (env *Env) EnsureSymbolIsNamespace(sym coretypes.Symbol) *Namespace {
	if sym.NamespaceKey() != nil {
		panic(coretypes.RuntimeError("Namespace's name cannot be qualified: " + sym.ToString(false)))
	}
	nameKey := sym.NameKey()
	nsRWMu.RLock()
	ns := env.Namespaces[nameKey]
	nsRWMu.RUnlock()
	if ns != nil {
		return ns
	}
	nsRWMu.Lock()
	if env.Namespaces[nameKey] == nil {
		env.Namespaces[nameKey] = NewNamespace(sym)
	}
	ns = env.Namespaces[nameKey]
	nsRWMu.Unlock()
	return ns
}

func (env *Env) EnsureSymbolIsLib(sym coretypes.Symbol) *Namespace {
	ns := env.EnsureSymbolIsNamespace(sym)
	env.libs.Value.(*corecollections.MapSet).Add(sym)
	return ns
}

func (env *Env) NamespaceFor(ns *Namespace, s coretypes.Symbol) *Namespace {
	var res *Namespace
	if s.NamespaceKey() == nil {
		res = ns
	} else {
		nsKey := s.NamespaceKey()
		res = ns.aliases[nsKey]
		if res == nil {
			nsRWMu.RLock()
			res = env.Namespaces[nsKey]
			nsRWMu.RUnlock()
		}
	}
	if res != nil {
		res.MaybeLazy("NamespaceFor")
	}
	return res
}

func (env *Env) ResolveIn(n *Namespace, s coretypes.Symbol) (*Var, bool) {
	ns := env.NamespaceFor(n, s)
	if ns == nil {
		return nil, false
	}
	if v, ok := ns.mappings[s.NameKey()]; ok {
		traceSymbolResolve(ns, s, true)
		return v, true
	}
	if s.Equals(env.IN_NS_VAR.name) {
		traceSymbolResolve(ns, s, true)
		return env.IN_NS_VAR, true
	}
	if s.Equals(env.NS_VAR.name) {
		traceSymbolResolve(ns, s, true)
		return env.NS_VAR, true
	}
	return nil, false
}

func (env *Env) Resolve(s coretypes.Symbol) (*Var, bool) {
	return env.ResolveIn(env.CurrentNamespace(), s)
}

func (env *Env) FindNamespace(s coretypes.Symbol) *Namespace {
	if s.NamespaceKey() != nil {
		return nil
	}
	nsRWMu.RLock()
	ns := env.Namespaces[s.NameKey()]
	nsRWMu.RUnlock()
	if ns != nil {
		ns.MaybeLazy("FindNameSpace")
	}
	return ns
}

func (env *Env) RemoveNamespace(s coretypes.Symbol) *Namespace {
	if s.NamespaceKey() != nil {
		return nil
	}
	if s.Equals(SYMBOLS.joker_core) {
		panic(coretypes.RuntimeError("Cannot remove core namespace"))
	}
	nameKey := s.NameKey()
	nsRWMu.Lock()
	ns := env.Namespaces[nameKey]
	delete(env.Namespaces, nameKey)
	nsRWMu.Unlock()
	return ns
}

func (env *Env) ResolveSymbol(s coretypes.Symbol) coretypes.Symbol {
	if corestr.HasNamespaceSeparator(s.Name(), '.') {
		return s
	}
	nameKey := s.NameKey()
	nsKey := s.NamespaceKey()
	if nsKey == nil && TYPES.Contains(nameKey) {
		return s
	}
	currentNs := env.CurrentNamespace()
	if nsKey != nil {
		ns := env.NamespaceFor(currentNs, s)
		if ns == nil || ns.Name.NameKey() == nsKey {
			if ns != nil {
				ns.isUsed = true
				ns.isGloballyUsed = true
			}
			return s
		}
		ns.isUsed = true
		ns.isGloballyUsed = true
		return coretypes.MakeSymbolFromKeys(ns.Name.NameKey(), nameKey)
	}
	vr, ok := currentNs.mappings[nameKey]
	if !ok {
		return coretypes.MakeSymbolFromKeys(currentNs.Name.NameKey(), nameKey)
	}
	vr.isUsed = true
	vr.isGloballyUsed = true
	vr.ns.isUsed = true
	vr.ns.isGloballyUsed = true
	return coretypes.MakeSymbolFromKeys(vr.ns.Name.NameKey(), vr.name.NameKey())
}

func init() {
	GLOBAL_ENV.SetCurrentNamespace(GLOBAL_ENV.EnsureSymbolIsNamespace(coretypes.MakeSymbol(STRINGS.Intern, "user")))
}

// ---- goroutine_rt.go ----
// goroutine_rt.go — Per-goroutine runtime state, replacing the GIL.
//
// Each goroutine gets its own callstack and currentExpr for error reporting.
// The main goroutine uses a fast path (zero overhead when no spawned goroutines).
// Spawned goroutines use a sync.Map keyed by goroutine ID.
//
// With the GIL removed:
// - Immutable data structures (vectors, maps, lists, seqs) are already thread-safe.
// - Atoms use a per-atom mutex for swap!/reset!/compare-and-set! correctness.
// - Var.Value writes (def, alter-var-root) are rare and only safe from the main
//   goroutine or under user coordination (same semantics as Clojure's JVM runtime).
// - Namespace map mutations are protected by nsRWMu.

// goroutineRT holds per-goroutine interpreter state.
type goroutineRT struct {
	callstack   *Callstack
	currentExpr Expr
}

var (
	// mainRT is the default runtime for the main goroutine (hot path).
	mainRT = goroutineRT{
		callstack: &Callstack{frames: make([]Frame, 0, 50)},
	}
	goroutineState *corert.GoRTPool

	// nsRWMu protects GLOBAL_ENV.Namespaces map mutations.
	nsRWMu sync.RWMutex
)

func init() {
	goroutineState = corert.NewGoRTPool(corert.GoID, &mainRT)
}

// currentGRT returns the goroutineRT for the current goroutine.
// HOT PATH: When no spawned goroutines exist (the common case for
// single-threaded execution), returns &mainRT with a single atomic load.
func currentGRT() *goroutineRT {
	return goroutineState.Current().(*goroutineRT)
}

// registerGoroutineRT sets up a new goroutineRT for the current goroutine.
// Called once at goroutine start.
func registerGoroutineRT() *goroutineRT {
	grt := &goroutineRT{callstack: &Callstack{frames: make([]Frame, 0, 20)}}
	goroutineState.Register(grt)
	return grt
}

// unregisterGoroutineRT removes the current goroutine's state.
// Called once at goroutine exit.
func unregisterGoroutineRT() {
	goroutineState.Unregister()
}

// ---- ns.go ----

type (
	Namespace struct {
		coretypes.MetaHolder
		Name           coretypes.Symbol
		Lazy           func()
		mappings       map[*string]*Var
		aliases        map[*string]*Namespace
		isUsed         bool
		isGloballyUsed bool
		hash           uint32
	}
)

func (ns *Namespace) ToString(escape bool) string {
	return ns.Name.ToString(escape)
}

func (ns *Namespace) Print(w io.Writer, printReadably bool) {
	fmt.Fprint(w, "#object[Namespace \""+ns.Name.ToString(true)+"\"]")
}

func (ns *Namespace) Equals(other interface{}) bool {
	return ns == other
}

func (ns *Namespace) GetInfo() *coretypes.ObjectInfo {
	return nil
}

func (ns *Namespace) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	return ns
}

func (ns *Namespace) GetType() *coretypes.Type {
	return TYPE.Namespace
}

func (ns *Namespace) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *ns
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (ns *Namespace) ResetMeta(newMeta coretypes.Map) coretypes.Map {
	ns.Meta = newMeta
	return ns.Meta
}

func (ns *Namespace) AlterMeta(fn coretypes.Callable, args []coretypes.Object) coretypes.Map {
	return AlterMeta(&ns.MetaHolder, fn, args)
}

func (ns *Namespace) Hash() uint32 {
	return ns.hash
}

func (ns *Namespace) MaybeLazy(doc string) {
	if ns.Lazy != nil {
		lazyFn := ns.Lazy
		ns.Lazy = nil
		lazyFn()
		if VerbosityLevel > 0 {
			fmt.Fprintf(Stderr, "NamespaceFor: Lazily initialized %s for %s\n", ns.Name.Name(), doc)
		}
	}
}

const nsHashMask uint32 = 0x90569f6f

func NewNamespace(sym coretypes.Symbol) *Namespace {
	return &Namespace{
		Name:     sym,
		mappings: make(map[*string]*Var),
		aliases:  make(map[*string]*Namespace),
		hash:     sym.Hash() ^ nsHashMask,
	}
}

func (ns *Namespace) Refer(sym coretypes.Symbol, vr *Var) *Var {
	if sym.NamespaceKey() != nil {
		panic(RT.NewError("Can't intern namespace-qualified symbol " + sym.ToString(false)))
	}
	ns.mappings[sym.NameKey()] = vr
	return vr
}

func (ns *Namespace) ReferAll(other *Namespace) {
	for name, vr := range other.mappings {
		if !vr.isPrivate {
			ns.mappings[name] = vr
		}
	}
}

func (ns *Namespace) InternFake(sym coretypes.Symbol) *Var {
	vr := ns.Intern(sym)
	vr.isFake = true
	return vr
}

func (ns *Namespace) Intern(sym coretypes.Symbol) *Var {
	if sym.NamespaceKey() != nil {
		panic(RT.NewError("Can't intern namespace-qualified symbol " + sym.ToString(false)))
	}
	nameKey := sym.NameKey()
	if LINTER_MODE {
		if LINTER_TYPES[nameKey] {
			msg := fmt.Sprintf("Expecting var, but %s is a type", sym.Name())
			pos := sym.GetInfo().Pos()
			printParseWarning(pos, msg)
		}
	}
	sym = sym.WithMeta(nil).(coretypes.Symbol)
	existingVar, ok := ns.mappings[nameKey]
	if !ok {
		newVar := &Var{
			ns:   ns,
			name: sym,
		}
		ns.mappings[nameKey] = newVar
		return newVar
	}
	if existingVar.ns != ns {
		if existingVar.ns.Name.Equals(SYMBOLS.joker_core) {
			newVar := &Var{
				ns:   ns,
				name: sym,
			}
			ns.mappings[nameKey] = newVar
			if !corestr.HasJokerNamespacePrefix(ns.Name.Name()) {
				printParseWarning(GetPosition(sym), fmt.Sprintf("WARNING: %s already refers to: %s in namespace %s, being replaced by: %s\n",
					sym.ToString(false), existingVar.ToString(false), ns.Name.ToString(false), newVar.ToString(false)))
			}
			return newVar
		}
		panic(RT.NewErrorWithPos(fmt.Sprintf("WARNING: %s already refers to: %s in namespace %s",
			sym.ToString(false), existingVar.ToString(false), ns.ToString(false)), sym.GetInfo().Pos()))
	}
	if LINTER_MODE && existingVar.expr != nil && !existingVar.ns.Name.Equals(SYMBOLS.joker_core) {
		if !isDeclaredInConfig(existingVar) {
			if sym.GetInfo() == nil {
				printParseWarning(existingVar.GetInfo().Pos(), "Subsequent duplicate def of "+existingVar.ToString(false))
			} else {
				printParseWarning(sym.GetInfo().Pos(), "Duplicate def of "+existingVar.ToString(false))
			}
		}
	}
	return existingVar
}

func isDeclaredInConfig(vr *Var) bool {
	m := vr.GetMeta()
	if m == nil {
		return false
	}
	ok, v := m.Get(KEYWORDS.file)
	if !ok {
		return false
	}
	s, ok := v.(coretypes.String)
	if !ok {
		return false
	}
	return corestr.IsJokerdPath(s.S)
}

func (ns *Namespace) InternVar(name string, val coretypes.Object, meta *corecollections.ArrayMap) *Var {
	vr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, name))
	vr.Value = val
	meta.Add(KEYWORDS.ns, ns)
	meta.Add(KEYWORDS.name, vr.name)
	vr.Meta = meta
	return vr
}

func (ns *Namespace) AddAlias(alias coretypes.Symbol, namespace *Namespace) {
	if alias.NamespaceKey() != nil {
		panic(RT.NewError("Alias can't be namespace-qualified"))
	}
	aliasKey := alias.NameKey()
	existing := ns.aliases[aliasKey]
	if existing != nil && existing != namespace {
		msg := "Alias " + alias.ToString(false) + " already exists in namespace " + ns.Name.ToString(false) + ", aliasing " + existing.Name.ToString(false)
		if LINTER_MODE {
			printParseError(GetPosition(alias), msg)
			return
		}
		panic(RT.NewError(msg))
	}
	ns.aliases[aliasKey] = namespace
}

func (ns *Namespace) Resolve(name string) *Var {
	return ns.mappings[STRINGS.Intern(name)]
}

func (ns *Namespace) Mappings() map[*string]*Var {
	return ns.mappings
}

func (ns *Namespace) Aliases() map[*string]*Namespace {
	return ns.aliases
}

// ---- z_doc_meta.go ----
// z_doc_meta.go — metadata hygiene for native/runtime-installed Vars.
//
// Most public vars are generated from .joke sources and carry rich metadata.
// A few runtime-installed compatibility helpers are installed directly from Go;
// make sure they still have enough metadata for doc generation and lint-style
// checks instead of surfacing as noisy <internal> warnings.

func init() {
	fillNativeVarMetadata()
}

func fillNativeVarMetadata() {
	if GLOBAL_ENV == nil {
		return
	}
	nsRWMu.RLock()
	namespaces := make([]*Namespace, 0, len(GLOBAL_ENV.Namespaces))
	for _, ns := range GLOBAL_ENV.Namespaces {
		namespaces = append(namespaces, ns)
	}
	nsRWMu.RUnlock()
	for _, ns := range namespaces {
		for _, vr := range ns.mappings {
			if vr == nil || vr.ns != ns || vr.isFake {
				continue
			}
			m, _ := vr.Meta.(*corecollections.ArrayMap)
			if m == nil {
				m = corecollections.EmptyArrayMap()
				if vr.Meta != nil {
					for it := vr.Meta.Iter(); it.HasNext(); {
						p := it.Next()
						m.Add(p.Key, p.Value)
					}
				}
				vr.Meta = m
			}
			if ok, _ := m.Get(KEYWORDS.ns); !ok {
				m.Add(KEYWORDS.ns, ns)
			}
			if ok, _ := m.Get(KEYWORDS.name); !ok {
				m.Add(KEYWORDS.name, vr.name)
			}
			if vr.isPrivate {
				if ok, _ := m.Get(KEYWORDS.private); !ok {
					m.Add(KEYWORDS.private, coretypes.Boolean{B: true})
				}
				continue
			}
			if ok, _ := m.Get(KEYWORDS.added); !ok {
				m.Add(KEYWORDS.added, coretypes.MakeString("1.0"))
			}
			if ok, _ := m.Get(KEYWORDS.doc); !ok {
				m.Add(KEYWORDS.doc, coretypes.MakeString("Native runtime helper installed by go-joker."))
			}
		}
	}
}
