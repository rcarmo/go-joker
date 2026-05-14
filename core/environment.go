package core

import (
	"io"
	"os"

	"github.com/rcarmo/go-joker/core/osutil"
	corestr "github.com/rcarmo/go-joker/core/string"
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
		Features      Set
	}
)

func versionMap() Map {
	res := collectionConstruction.EmptyArrayMap()
	major, minor, incremental := corestr.ParseVersionTriplet(VERSION)
	res.Add(MakeKeyword("major"), Int{I: int(major)})
	res.Add(MakeKeyword("minor"), Int{I: int(minor)})
	res.Add(MakeKeyword("incremental"), Int{I: int(incremental)})
	return res
}

func (env *Env) SetEnvArgs(newArgs []string) {
	args := collectionConstruction.EmptyArrayVector()
	for _, arg := range newArgs {
		args.Append(MakeString(arg))
	}
	if args.Count() > 0 {
		env.args.Value = args.Seq()
	} else {
		env.args.Value = NIL
	}
}

/*
This runs after invariant initialization, which includes calling

	NewEnv().  NOTE: Any changes to the list of run-time
	initializations must be reflected in gen/codegen/main.go.
*/
func (env *Env) SetClassPath(cp string) {
	cpVec := collectionConstruction.EmptyArrayVector()
	for _, cpelem := range osutil.ClassPathElements(cp) {
		cpVec.Append(MakeString(cpelem))
	}
	env.classPath.Value = cpVec
}

/*
This runs after invariant initialization, which includes calling

	NewEnv().  NOTE: Any changes to the list of run-time
	initializations must be reflected in gen/codegen/main.go.
*/
func (env *Env) InitEnv(stdin io.Reader, stdout, stderr io.Writer, args []string) {
	env.stdin.Value = MakeBufferedReader(stdin)
	env.stdout.Value = MakeIOWriter(stdout)
	env.stderr.Value = MakeIOWriter(stderr)
	// Keep constantly capture-correct even when the evaluator's fixed-arity
	// call fast paths are active; the core.joke closure shape is sensitive to
	// local frame reuse in this optimized fork.
	if vr := env.CoreNamespace.Resolve("constantly"); vr != nil {
		vr.Value = Proc{Name: "procConstantly", Fn: func(args []Object) Object {
			CheckArity(args, 1, 1)
			x := args[0]
			return Proc{Name: "procConstantlyValue", Fn: func(_ []Object) Object { return x }}
		}}
	}
	env.SetEnvArgs(args)
}

func (env *Env) SetStdIO(stdin, stdout, stderr Object) {
	env.stdin.Value = stdin
	env.stdout.Value = stdout
	env.stderr.Value = stderr
}

func (env *Env) StdIO() (stdin, stdout, stderr Object) {
	return env.stdin.Value, env.stdout.Value, env.stderr.Value
}

/*
This runs after invariant initialization, which includes calling

	NewEnv().  NOTE: Any changes to the list of run-time
	initializations must be reflected in gen/codegen/main.go.
*/
func (env *Env) SetMainFilename(filename string) {
	env.MainFile.Value = MakeString(filename)
}

/*
This runs after invariant initialization, which includes calling

	NewEnv().  NOTE: Any changes to the list of run-time
	initializations must be reflected in gen/codegen/main.go.
*/
func (env *Env) SetFilename(obj Object) {
	env.file.Value = obj
}

func (env *Env) IsStdIn(obj Object) bool {
	return env.stdin.Value == obj
}

func (env *Env) CurrentNamespace() *Namespace {
	return EnsureObjectIsNamespace(env.ns.Value, "")
}

func (env *Env) SetCurrentNamespace(ns *Namespace) {
	env.ns.Value = ns
}

func (env *Env) EnsureSymbolIsNamespace(sym Symbol) *Namespace {
	if sym.ns != nil {
		panic(RT.NewError("Namespace's name cannot be qualified: " + sym.ToString(false)))
	}
	nsRWMu.RLock()
	ns := env.Namespaces[sym.name]
	nsRWMu.RUnlock()
	if ns != nil {
		return ns
	}
	nsRWMu.Lock()
	// Double-check under write lock.
	if env.Namespaces[sym.name] == nil {
		env.Namespaces[sym.name] = NewNamespace(sym)
	}
	ns = env.Namespaces[sym.name]
	nsRWMu.Unlock()
	return ns
}

func (env *Env) EnsureSymbolIsLib(sym Symbol) *Namespace {
	ns := env.EnsureSymbolIsNamespace(sym)
	env.libs.Value.(*MapSet).Add(sym)
	return ns
}

func (env *Env) NamespaceFor(ns *Namespace, s Symbol) *Namespace {
	var res *Namespace
	if s.ns == nil {
		res = ns
	} else {
		res = ns.aliases[s.ns]
		if res == nil {
			nsRWMu.RLock()
			res = env.Namespaces[s.ns]
			nsRWMu.RUnlock()
		}
	}
	if res != nil {
		res.MaybeLazy("NamespaceFor")
	}
	return res
}

func (env *Env) ResolveIn(n *Namespace, s Symbol) (*Var, bool) {
	ns := env.NamespaceFor(n, s)
	if ns == nil {
		return nil, false
	}
	if v, ok := ns.mappings[s.name]; ok {
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

func (env *Env) Resolve(s Symbol) (*Var, bool) {
	return env.ResolveIn(env.CurrentNamespace(), s)
}

func (env *Env) FindNamespace(s Symbol) *Namespace {
	if s.ns != nil {
		return nil
	}
	nsRWMu.RLock()
	ns := env.Namespaces[s.name]
	nsRWMu.RUnlock()
	if ns != nil {
		ns.MaybeLazy("FindNameSpace")
	}
	return ns
}

func (env *Env) RemoveNamespace(s Symbol) *Namespace {
	if s.ns != nil {
		return nil
	}
	if s.Equals(SYMBOLS.joker_core) {
		panic(RT.NewError("Cannot remove core namespace"))
	}
	nsRWMu.Lock()
	ns := env.Namespaces[s.name]
	delete(env.Namespaces, s.name)
	nsRWMu.Unlock()
	return ns
}

func (env *Env) ResolveSymbol(s Symbol) Symbol {
	if corestr.HasNamespaceSeparator(*s.name, '.') {
		return s
	}
	if s.ns == nil && TYPES[s.name] != nil {
		return s
	}
	currentNs := env.CurrentNamespace()
	if s.ns != nil {
		ns := env.NamespaceFor(currentNs, s)
		if ns == nil || ns.Name.name == s.ns {
			if ns != nil {
				ns.isUsed = true
				ns.isGloballyUsed = true
			}
			return s
		}
		ns.isUsed = true
		ns.isGloballyUsed = true
		return Symbol{
			name: s.name,
			ns:   ns.Name.name,
		}
	}
	vr, ok := currentNs.mappings[s.name]
	if !ok {
		return Symbol{
			name: s.name,
			ns:   currentNs.Name.name,
		}
	}
	vr.isUsed = true
	vr.isGloballyUsed = true
	vr.ns.isUsed = true
	vr.ns.isGloballyUsed = true
	return Symbol{
		name: vr.name.name,
		ns:   vr.ns.Name.name,
	}
}

func init() {
	GLOBAL_ENV.SetCurrentNamespace(GLOBAL_ENV.EnsureSymbolIsNamespace(MakeSymbol("user")))
}
