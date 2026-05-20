package core

// protocol.go — Protocol support for Clojure parity.
//
// Implements:
// - defprotocol: defines a protocol with method signatures
// - extend-type: extends a protocol to a Go type
// - satisfies?: checks if a value satisfies a protocol
//
// Protocols are represented as a Protocol object holding method name → dispatch map.
// Each method dispatch map maps Go type names to implementing functions.

import (
	"fmt"
	"sync"
	"unsafe"

	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	"github.com/rcarmo/go-joker/core/hashutil"
)

// Protocol represents a Clojure-style protocol.
type Protocol struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	name    coretypes.Symbol
	methods map[string]*ProtocolMethod // method name → method descriptor
	ns      *Namespace
}

// ProtocolMethod holds one method's signature and dispatch table.
type ProtocolMethod struct {
	name        string
	arities     []int              // accepted arities (including 'this')
	dispatch    sync.Map           // type name (string) → coretypes.Callable
	defaultImpl coretypes.Callable // nil or default implementation
}

func (p *Protocol) ToString(escape bool) string {
	return fmt.Sprintf("#object[Protocol %s]", p.name.ToString(false))
}

func (p *Protocol) Equals(other interface{}) bool {
	if o, ok := other.(*Protocol); ok {
		return p == o
	}
	return false
}

func (p *Protocol) GetType() *coretypes.Type { return TYPE.Fn }
func (p *Protocol) Hash() uint32             { return hashutil.Ptr(uintptr(unsafe.Pointer(p))) }

func (p *Protocol) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *p
	res.Info = info
	return &res
}

func (p *Protocol) WithMeta(m coretypes.Map) coretypes.Object {
	res := *p
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}

// lookupMethod finds the implementation of a method for a given object.
func (pm *ProtocolMethod) lookupMethod(obj coretypes.Object) coretypes.Callable {
	typeName := typeNameOf(obj)
	if fn, ok := pm.dispatch.Load(typeName); ok {
		return fn.(coretypes.Callable)
	}
	// Try "coretypes.Object" catch-all
	if fn, ok := pm.dispatch.Load("coretypes.Object"); ok {
		return fn.(coretypes.Callable)
	}
	if pm.defaultImpl != nil {
		return pm.defaultImpl
	}
	return nil
}

// typeNameOf returns the dispatch type name for an object.
func typeNameOf(obj coretypes.Object) string {
	if obj == nil {
		return "nil"
	}
	switch obj := obj.(type) {
	case Nil:
		return "nil"
	case coretypes.Int:
		return "Int"
	case coretypes.Double:
		return "Double"
	case coretypes.Boolean:
		return "Boolean"
	case coretypes.String:
		return "String"
	case coretypes.Char:
		return "Char"
	case coretypes.Keyword:
		return "Keyword"
	case coretypes.Symbol:
		return "Symbol"
	case *coretypes.Regex:
		return "Regex"
	case *corecollections.Vector:
		return "corecollections.Vector"
	case *corecollections.ArrayVector:
		return "corecollections.Vector"
	case *corecollections.ArrayMap:
		return "Map"
	case *corecollections.HashMap:
		return "Map"
	case *corecollections.MapSet:
		return "Set"
	case *corecollections.List:
		return "corecollections.List"
	case *corecollections.LazySeq:
		return "corecollections.LazySeq"
	case *corecollections.ConsSeq:
		return "coretypes.Seq"
	case *corecollections.ArraySeq:
		return "coretypes.Seq"
	case *corecollections.MappingSeq:
		return "coretypes.Seq"
	case *Fn:
		return "Fn"
	case Proc:
		return "Fn"
	case *corert.Atom:
		return "Atom"
	case *Record:
		return obj.rtype.Name
	default:
		return obj.GetType().ToString(false)
	}
}

// makeProtocolMethod creates a dispatch proc for a protocol method.
func makeProtocolMethodProc(proto *Protocol, methodName string, pm *ProtocolMethod) Proc {
	return Proc{
		Name: proto.name.ToString(false) + "/" + methodName,
		Fn: func(args []coretypes.Object) coretypes.Object {
			if len(args) == 0 {
				panic(coretypes.RuntimeError(fmt.Sprintf("Protocol method %s/%s called with no arguments",
					proto.name.ToString(false), methodName)))
			}
			impl := pm.lookupMethod(args[0])
			if impl == nil {
				panic(coretypes.RuntimeError(fmt.Sprintf("No implementation of protocol method %s/%s for type %s",
					proto.name.ToString(false), methodName, typeNameOf(args[0]))))
			}
			return impl.Call(args)
		},
	}
}

// DefineProtocol creates a new Protocol and installs its method vars.
// Called from the defprotocol special form handler.
func DefineProtocol(ns *Namespace, name coretypes.Symbol, methods []ProtocolMethodDef) *Protocol {
	proto := &Protocol{
		name:    name,
		methods: make(map[string]*ProtocolMethod),
		ns:      ns,
	}

	for _, mdef := range methods {
		pm := &ProtocolMethod{
			name:    mdef.Name,
			arities: mdef.Arities,
		}
		proto.methods[mdef.Name] = pm

		// Install the dispatch proc as a var in the protocol's namespace
		sym := coretypes.MakeSymbol(STRINGS.Intern, mdef.Name)
		vr := ns.Intern(sym)
		vr.Value = makeProtocolMethodProc(proto, mdef.Name, pm)
	}

	// Store the protocol itself
	protoVr := ns.Intern(name)
	protoVr.Value = proto

	return proto
}

// ProtocolMethodDef defines a method in a protocol.
type ProtocolMethodDef struct {
	Name    string
	Arities []int
}

// ExtendType extends a protocol's methods for a given type name.
func ExtendType(proto *Protocol, typeName string, impls map[string]coretypes.Callable) {
	for methodName, impl := range impls {
		pm, ok := proto.methods[methodName]
		if !ok {
			panic(coretypes.RuntimeError(fmt.Sprintf("No method %s in protocol %s",
				methodName, proto.name.ToString(false))))
		}
		pm.dispatch.Store(typeName, impl)
	}
}

// Satisfies checks if an object satisfies a protocol (has implementations for all methods).
func Satisfies(proto *Protocol, obj coretypes.Object) bool {
	typeName := typeNameOf(obj)
	for _, pm := range proto.methods {
		if _, ok := pm.dispatch.Load(typeName); !ok {
			if _, ok := pm.dispatch.Load("coretypes.Object"); !ok {
				if pm.defaultImpl == nil {
					return false
				}
			}
		}
	}
	return true
}

// ---- protocol_init.go ----
// protocol_init.go — Register defprotocol, extend-type, extend-protocol, satisfies?
// as runtime procs/macros in the core namespace.

func init() {
	registerProtocolProcs()
}

func registerProtocolProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// satisfies? — checks if an object satisfies a protocol
	satVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "satisfies?"))
	satVr.Value = Proc{Name: "procSatisfiesQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		proto, ok := args[0].(*Protocol)
		if !ok {
			panic(coretypes.RuntimeError("First argument to satisfies? must be a Protocol"))
		}
		return coretypes.MakeBoolean(Satisfies(proto, args[1]))
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "satisfies?"), satVr)

	// extends? — checks if a type extends a protocol
	extVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "extends?"))
	extVr.Value = Proc{Name: "procExtendsQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		proto, ok := args[0].(*Protocol)
		if !ok {
			panic(coretypes.RuntimeError("First argument to extends? must be a Protocol"))
		}
		return coretypes.MakeBoolean(Satisfies(proto, args[1]))
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "extends?"), extVr)

	// __defprotocol — internal helper called by defprotocol macro
	// Args: [protocol-name-string method1-name arity1 method2-name arity2 ...]
	defProtoVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "__defprotocol"))
	defProtoVr.Value = Proc{Name: "procDefProtocolInternal", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 1 {
			panic(coretypes.RuntimeError("__defprotocol requires at least a name"))
		}
		name := coretypes.EnsureObjectIsSymbol(args[0], "defprotocol name must be a symbol")

		var methods []ProtocolMethodDef
		i := 1
		for i < len(args) {
			methodName := coretypes.EnsureObjectIsString(args[i], "method name must be a string").S
			i++
			if i >= len(args) {
				break
			}
			arity := coretypes.EnsureObjectIsInt(args[i], "method arity must be an int").I
			i++
			methods = append(methods, ProtocolMethodDef{
				Name:    methodName,
				Arities: []int{arity},
			})
		}

		currentNs := GLOBAL_ENV.CurrentNamespace()
		proto := DefineProtocol(currentNs, name, methods)
		return proto
	}}
	defProtoVr.isPrivate = true
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "__defprotocol"), defProtoVr)

	// __extend-type — internal helper called by extend-type macro
	// Args: [protocol type-name-string method1-name fn1 method2-name fn2 ...]
	extTypeVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "__extend-type"))
	extTypeVr.Value = Proc{Name: "procExtendTypeInternal", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			panic(coretypes.RuntimeError("__extend-type requires protocol and type-name"))
		}
		proto, ok := args[0].(*Protocol)
		if !ok {
			panic(coretypes.RuntimeError("First argument to __extend-type must be a Protocol"))
		}
		typeName := coretypes.EnsureObjectIsString(args[1], "type name must be a string").S

		if len(args[2:])%2 != 0 {
			panic(coretypes.RuntimeError("__extend-type method implementations must be name/function pairs"))
		}
		impls := make(map[string]coretypes.Callable)
		i := 2
		for i+1 < len(args) {
			methodName := coretypes.EnsureObjectIsString(args[i], "method name must be a string").S
			fn := coretypes.EnsureObjectIsCallable(args[i+1], "method implementation must be callable, got %s")
			impls[methodName] = fn
			i += 2
		}

		ExtendType(proto, typeName, impls)
		return NIL
	}}
	extTypeVr.isPrivate = true
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "__extend-type"), extTypeVr)
}

// ---- public_forms.go ----
// public_forms.go — Public macro forms for protocols and records.
//
// The runtime helpers (__defprotocol, __extend-type, __defrecord) are useful
// for bootstrapping and tests, but Clojure users expect public forms. These
// macros expand to the internal helpers and are registered early so the parser
// can resolve them before user code is parsed.

func init() {
	registerPublicParityMacros()
}

func registerPublicParityMacros() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}
	installMacro(ns, "defprotocol", macroDefProtocol)
	installMacro(ns, "extend-type", macroExtendType)
	installMacro(ns, "extend-protocol", macroExtendProtocol)
	installMacro(ns, "defrecord", macroDefRecord)
}

func installMacro(ns *Namespace, name string, fn func([]coretypes.Object) coretypes.Object) {
	sym := coretypes.MakeSymbol(STRINGS.Intern, name)
	vr := ns.Intern(sym)
	vr.Value = Proc{Name: "macro" + name, Fn: fn}
	vr.isMacro = true
	referToUser(sym, vr)
}

func listObjs(objs ...coretypes.Object) *corecollections.List {
	return corecollections.NewListFrom(objs...)
}
func quoteObj(obj coretypes.Object) *corecollections.List {
	return listObjs(coretypes.MakeSymbol(STRINGS.Intern, "quote"), obj)
}
func doObj(forms ...coretypes.Object) *corecollections.List {
	return corecollections.NewListFrom(append([]coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "do")}, forms...)...)
}

func macroDefProtocol(args []coretypes.Object) coretypes.Object {
	// macro args: &form, &env, name, method...
	if len(args) < 3 {
		panic(coretypes.RuntimeError("defprotocol requires a name"))
	}
	name, ok := args[2].(coretypes.Symbol)
	if !ok {
		panic(coretypes.RuntimeError("defprotocol name must be a symbol"))
	}
	forms := []coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "__defprotocol"), quoteObj(name)}
	for _, raw := range args[3:] {
		seqable, ok := raw.(coretypes.Seqable)
		if !ok {
			continue // docstrings/options are ignored by the compact runtime protocol helper
		}
		s := seqable.Seq()
		if s.IsEmpty() {
			continue
		}
		mname, ok := s.First().(coretypes.Symbol)
		if !ok {
			continue
		}
		s = s.Rest()
		if s.IsEmpty() {
			continue
		}
		argv, ok := s.First().(coretypes.Counted)
		if !ok {
			continue
		}
		forms = append(forms, coretypes.String{S: mname.ToString(false)}, coretypes.Int{I: argv.Count()})
	}
	return corecollections.NewListFrom(forms...)
}

func macroExtendType(args []coretypes.Object) coretypes.Object {
	// (extend-type Type Proto (method [args] body...) Proto2 ...)
	if len(args) < 5 {
		panic(coretypes.RuntimeError("extend-type requires a type, protocol, and method implementations"))
	}
	typeName := macroTypeName(args[2])
	forms := make([]coretypes.Object, 0)
	i := 3
	for i < len(args) {
		proto := args[i]
		i++
		call := []coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "__extend-type"), proto, coretypes.String{S: typeName}}
		for i < len(args) {
			if _, isProto := args[i].(coretypes.Symbol); isProto && i+1 < len(args) {
				if _, nextIsMethod := args[i+1].(coretypes.Seqable); nextIsMethod {
					break
				}
			}
			method, ok := args[i].(coretypes.Seqable)
			if !ok {
				break
			}
			s := method.Seq()
			if s.IsEmpty() {
				i++
				continue
			}
			mname, ok := s.First().(coretypes.Symbol)
			if !ok {
				i++
				continue
			}
			fnTail := corecollections.ToSlice(s.Rest())
			fnForm := corecollections.NewListFrom(append([]coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "fn")}, fnTail...)...)
			call = append(call, coretypes.String{S: mname.ToString(false)}, fnForm)
			i++
		}
		forms = append(forms, corecollections.NewListFrom(call...))
	}
	return doObj(forms...)
}

func macroExtendProtocol(args []coretypes.Object) coretypes.Object {
	// (extend-protocol Proto Type (method [args] body...) Type2 ...)
	if len(args) < 5 {
		panic(coretypes.RuntimeError("extend-protocol requires a protocol, type, and method implementations"))
	}
	proto := args[2]
	forms := make([]coretypes.Object, 0)
	i := 3
	for i < len(args) {
		typeName := macroTypeName(args[i])
		i++
		call := []coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "__extend-type"), proto, coretypes.String{S: typeName}}
		for i < len(args) {
			method, ok := args[i].(coretypes.Seqable)
			if !ok {
				break
			}
			s := method.Seq()
			if s.IsEmpty() {
				i++
				continue
			}
			mname, ok := s.First().(coretypes.Symbol)
			if !ok {
				i++
				continue
			}
			fnTail := corecollections.ToSlice(s.Rest())
			fnForm := corecollections.NewListFrom(append([]coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "fn")}, fnTail...)...)
			call = append(call, coretypes.String{S: mname.ToString(false)}, fnForm)
			i++
			// Stop if the next form looks like a type followed by methods. In practice
			// a new type is a symbol/string/keyword and a method implementation is a list.
			if i < len(args) {
				if _, ok := args[i].(coretypes.Seqable); !ok {
					break
				}
			}
		}
		forms = append(forms, corecollections.NewListFrom(call...))
	}
	return doObj(forms...)
}

func macroDefRecord(args []coretypes.Object) coretypes.Object {
	// (defrecord Name [fields] Protocol (method [args] body...) ...)
	if len(args) < 4 {
		panic(coretypes.RuntimeError("defrecord requires a name and fields vector"))
	}
	name, ok := args[2].(coretypes.Symbol)
	if !ok {
		panic(coretypes.RuntimeError("defrecord name must be a symbol"))
	}
	fieldsSeq, ok := args[3].(coretypes.Seqable)
	if !ok {
		panic(coretypes.RuntimeError("defrecord fields must be seqable"))
	}
	defCall := []coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "__defrecord"), quoteObj(name)}
	for s := fieldsSeq.Seq(); !s.IsEmpty(); s = s.Rest() {
		field, ok := s.First().(coretypes.Symbol)
		if !ok {
			panic(coretypes.RuntimeError("defrecord field must be a symbol"))
		}
		defCall = append(defCall, coretypes.String{S: field.ToString(false)})
	}
	forms := []coretypes.Object{corecollections.NewListFrom(defCall...)}
	if len(args) > 4 {
		// Reuse extend-type semantics with the record's dispatch type name.
		extendArgs := append([]coretypes.Object{args[0], args[1], name}, args[4:]...)
		forms = append(forms, macroExtendType(extendArgs))
	}
	return doObj(forms...)
}

func macroTypeName(obj coretypes.Object) string {
	switch t := obj.(type) {
	case coretypes.Symbol:
		return t.ToString(false)
	case coretypes.String:
		return t.S
	case coretypes.Keyword:
		return t.ToString(false)[1:]
	default:
		return obj.ToString(false)
	}
}
