package core

import (
	"fmt"
	"io"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	corestr "github.com/rcarmo/go-joker/core/types/string"
)

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
