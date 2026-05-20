package core

import (
	"io"
	"os"

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
