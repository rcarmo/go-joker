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
	"strings"
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

// ---- record.go ----
// record.go — Record support for Clojure parity.
//
// A Record is a named, typed map with fixed fields plus optional extension fields.
// Records support:
// - Keyword access: (:field record)
// - get/assoc/dissoc (dissoc to extension fields only; dissoc of base field returns plain map)
// - coretypes.Equality by type + fields
// - Protocol satisfaction via extend-type with the record's type name

// Record is an instance of a RecordType.
type Record struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	rtype *coretypes.RecordType
	bases []coretypes.Object        // values for base fields (same order as rtype.fields)
	ext   *corecollections.ArrayMap // extension fields (nil if none)
}

func (r *Record) ToString(escape bool) string {
	var b strings.Builder
	b.WriteString("#")
	b.WriteString(r.rtype.Name)
	b.WriteString("{")
	first := true
	for i, fname := range r.rtype.Fields {
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(":")
		b.WriteString(fname)
		b.WriteString(" ")
		b.WriteString(r.bases[i].ToString(escape))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			if !first {
				b.WriteString(", ")
			}
			first = false
			b.WriteString(p.Key.ToString(escape))
			b.WriteString(" ")
			b.WriteString(p.Value.ToString(escape))
		}
	}
	b.WriteString("}")
	return b.String()
}

func (r *Record) Equals(other interface{}) bool {
	o, ok := other.(*Record)
	if !ok {
		return false
	}
	if r.rtype != o.rtype {
		return false
	}
	for i := range r.bases {
		if !r.bases[i].Equals(o.bases[i]) {
			return false
		}
	}
	// Compare extension fields
	if r.ext == nil && o.ext == nil {
		return true
	}
	if r.ext == nil || o.ext == nil {
		rCount := 0
		oCount := 0
		if r.ext != nil {
			rCount = r.ext.Count()
		}
		if o.ext != nil {
			oCount = o.ext.Count()
		}
		return rCount == 0 && oCount == 0
	}
	return r.ext.Equals(o.ext)
}

func (r *Record) GetType() *coretypes.Type { return TYPE.ArrayMap }
func (r *Record) Hash() uint32 {
	h := uint32(0x9e3779b9)
	for _, v := range r.bases {
		h = h*31 + v.Hash()
	}
	if r.ext != nil {
		h = h*31 + r.ext.Hash()
	}
	return h
}

func (r *Record) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := r.clone()
	res.Info = info
	return res
}

func (r *Record) WithMeta(m coretypes.Map) coretypes.Object {
	res := r.clone()
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return res
}

func (r *Record) clone() *Record {
	bases := make([]coretypes.Object, len(r.bases))
	copy(bases, r.bases)
	var ext *corecollections.ArrayMap
	if r.ext != nil {
		ext = r.ext.Clone()
	}
	return &Record{
		InfoHolder: r.InfoHolder,
		MetaHolder: r.MetaHolder,
		rtype:      r.rtype,
		bases:      bases,
		ext:        ext,
	}
}

// --- coretypes.Map interface ---

// Get implements coretypes.Gettable for keyword access.
func (r *Record) Get(key coretypes.Object) (bool, coretypes.Object) {
	if kw, ok := key.(coretypes.Keyword); ok {
		name := kw.ToString(false)[1:] // strip leading ":"
		if idx, ok := r.rtype.FieldIdx[name]; ok {
			return true, r.bases[idx]
		}
	}
	if r.ext != nil {
		return r.ext.Get(key)
	}
	return false, nil
}

// EntryAt returns a MapEntry for the given key.
func (r *Record) EntryAt(key coretypes.Object) coretypes.Object {
	if ok, v := r.Get(key); ok {
		av := corecollections.EmptyArrayVector().Conj(key).(*corecollections.ArrayVector).Conj(v).(*corecollections.ArrayVector)
		return av
	}
	return nil
}

// Assoc returns a new record with the key set to val.
// If key is a base field, returns a new record. Otherwise extends.
func (r *Record) Assoc(key, val coretypes.Object) coretypes.Associative {
	if kw, ok := key.(coretypes.Keyword); ok {
		name := kw.ToString(false)[1:]
		if idx, ok := r.rtype.FieldIdx[name]; ok {
			res := r.clone()
			res.bases[idx] = val
			return res
		}
	}
	res := r.clone()
	if res.ext == nil {
		res.ext = corecollections.EmptyArrayMap()
	}
	res.ext = res.ext.Assoc(key, val).(*corecollections.ArrayMap)
	return res
}

// Count returns the number of fields (base + extension).
func (r *Record) Count() int {
	n := len(r.bases)
	if r.ext != nil {
		n += r.ext.Count()
	}
	return n
}

// coretypes.Seq returns a sequence of MapEntry pairs.
func (r *Record) Seq() coretypes.Seq {
	entries := make([]coretypes.Object, 0, r.Count())
	for i, fname := range r.rtype.Fields {
		entries = append(entries, corecollections.NewVectorFrom(coretypes.MakeKeyword(STRINGS.Intern, fname), r.bases[i]))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			entries = append(entries, corecollections.NewVectorFrom(p.Key, p.Value))
		}
	}
	return &corecollections.ArraySeq{Arr: entries, Index: 0}
}

// Keys returns all keys.
func (r *Record) Keys() coretypes.Seq {
	keys := make([]coretypes.Object, 0, r.Count())
	for _, fname := range r.rtype.Fields {
		keys = append(keys, coretypes.MakeKeyword(STRINGS.Intern, fname))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			keys = append(keys, p.Key)
		}
	}
	return &corecollections.ArraySeq{Arr: keys, Index: 0}
}

// Vals returns all values.
func (r *Record) Vals() coretypes.Seq {
	vals := make([]coretypes.Object, 0, r.Count())
	vals = append(vals, r.bases...)
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			vals = append(vals, p.Value)
		}
	}
	return &corecollections.ArraySeq{Arr: vals, Index: 0}
}

// Conj adds a map entry to the record.
func (r *Record) Conj(obj coretypes.Object) coretypes.Conjable {
	switch v := obj.(type) {
	case *corecollections.Vector:
		if v.Count() != 2 {
			panic(coretypes.RuntimeError("corecollections.Vector arg to conj on record must be a pair"))
		}
		return r.Assoc(v.At(0), v.At(1)).(coretypes.Conjable)
	}
	panic(coretypes.RuntimeError(fmt.Sprintf("Cannot conj %s onto record", obj.GetType().ToString(false))))
}

// Call implements keyword-style access: (record :field)
func (r *Record) Call(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 2)
	ok, v := r.Get(args[0])
	if ok {
		return v
	}
	if len(args) == 2 {
		return args[1]
	}
	return NIL
}

// Merge merges a map into the record.
func (r *Record) Merge(other coretypes.Map) coretypes.Map {
	res := r.clone()
	for iter := other.Iter(); iter.HasNext(); {
		p := iter.Next()
		assocResult := res.Assoc(p.Key, p.Value)
		res = assocResult.(*Record)
	}
	return res
}

// Iter returns a map iterator.
func (r *Record) Iter() coretypes.MapIterator {
	return &recordIterator{r: r, idx: 0}
}

// Containskey
func (r *Record) ContainsKey(key coretypes.Object) bool {
	ok, _ := r.Get(key)
	return ok
}

// Without (dissoc) — dissoc of a base field returns a plain map
func (r *Record) Without(key coretypes.Object) coretypes.Map {
	if kw, ok := key.(coretypes.Keyword); ok {
		name := kw.ToString(false)[1:]
		if _, ok := r.rtype.FieldIdx[name]; ok {
			// Dissoc base field → degrade to plain map
			m := corecollections.EmptyArrayMap()
			for i, fname := range r.rtype.Fields {
				if fname != name {
					m.Add(coretypes.MakeKeyword(STRINGS.Intern, fname), r.bases[i])
				}
			}
			if r.ext != nil {
				for iter := r.ext.Iter(); iter.HasNext(); {
					p := iter.Next()
					m.Add(p.Key, p.Value)
				}
			}
			return m
		}
	}
	if r.ext != nil {
		res := r.clone()
		res.ext = res.ext.Without(key).(*corecollections.ArrayMap)
		return res
	}
	return r
}

type recordIterator struct {
	r       *Record
	idx     int
	extIter coretypes.MapIterator
}

func (it *recordIterator) HasNext() bool {
	if it.idx < len(it.r.rtype.Fields) {
		return true
	}
	if it.r.ext != nil {
		if it.extIter == nil {
			it.extIter = it.r.ext.Iter()
		}
		return it.extIter.HasNext()
	}
	return false
}

func (it *recordIterator) Next() *coretypes.Pair {
	if it.idx < len(it.r.rtype.Fields) {
		p := &coretypes.Pair{
			Key:   coretypes.MakeKeyword(STRINGS.Intern, it.r.rtype.Fields[it.idx]),
			Value: it.r.bases[it.idx],
		}
		it.idx++
		return p
	}
	if it.extIter == nil {
		it.extIter = it.r.ext.Iter()
	}
	return it.extIter.Next()
}

// NewRecord creates a new record instance.
func NewRecord(rtype *coretypes.RecordType, fields []coretypes.Object) *Record {
	if len(fields) != len(rtype.Fields) {
		panic(coretypes.RuntimeError(fmt.Sprintf("Wrong number of fields for record %s: expected %d, got %d",
			rtype.Name, len(rtype.Fields), len(fields))))
	}
	bases := make([]coretypes.Object, len(fields))
	copy(bases, fields)
	return &Record{rtype: rtype, bases: bases}
}

// ---- record_init.go ----
// record_init.go — Register __defrecord and record constructors.

func init() {
	registerRecordProcs()
}

func registerRecordProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// record? — always available
	recordQVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "record?"))
	recordQVr.Value = Proc{Name: "procRecordQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		_, ok := args[0].(*Record)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "record?"), recordQVr)

	// __defrecord — internal helper
	// Args: [record-name-symbol field1-string field2-string ...]
	// Returns: the RecordType, and installs:
	//   - ->RecordName constructor fn
	//   - map->RecordName factory fn
	defRecordVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "__defrecord"))
	defRecordVr.Value = Proc{Name: "procDefRecordInternal", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 1 {
			panic(coretypes.RuntimeError("__defrecord requires at least a name"))
		}
		name := coretypes.EnsureObjectIsSymbol(args[0], "defrecord name must be a symbol")
		nameStr := name.ToString(false)

		fields := make([]string, len(args)-1)
		for i := 1; i < len(args); i++ {
			fields[i-1] = coretypes.EnsureObjectIsString(args[i], "field name must be a string").S
		}

		rtype := coretypes.MakeRecordType(nameStr, fields)

		currentNs := GLOBAL_ENV.CurrentNamespace()

		// Install positional constructor: (->RecordName field1 field2 ...)
		ctorName := "->" + nameStr
		ctorVr := currentNs.Intern(coretypes.MakeSymbol(STRINGS.Intern, ctorName))
		ctorVr.Value = Proc{Name: "proc" + ctorName, Fn: func(ctorArgs []coretypes.Object) coretypes.Object {
			return NewRecord(rtype, ctorArgs)
		}}

		// Install map factory: (map->RecordName {:field1 v1 :field2 v2})
		mapCtorName := "map->" + nameStr
		mapCtorVr := currentNs.Intern(coretypes.MakeSymbol(STRINGS.Intern, mapCtorName))
		mapCtorVr.Value = Proc{Name: "proc" + mapCtorName, Fn: func(ctorArgs []coretypes.Object) coretypes.Object {
			runtimeCheckArity(ctorArgs, 1, 1)
			m := coretypes.EnsureObjectIsMap(ctorArgs[0], "map->"+nameStr+" requires a map argument")
			vals := make([]coretypes.Object, len(fields))
			for i, fname := range fields {
				kw := coretypes.MakeKeyword(STRINGS.Intern, fname)
				if ok, v := m.Get(kw); ok {
					vals[i] = v
				} else {
					vals[i] = NIL
				}
			}
			rec := NewRecord(rtype, vals)
			// Add any extra keys as extension fields
			for iter := m.Iter(); iter.HasNext(); {
				p := iter.Next()
				if kw, ok := p.Key.(coretypes.Keyword); ok {
					kwName := kw.ToString(false)[1:]
					if _, isBase := rtype.FieldIdx[kwName]; isBase {
						continue
					}
				}
				rec = rec.Assoc(p.Key, p.Value).(*Record)
			}
			return rec
		}}

		return NIL
	}}
	defRecordVr.isPrivate = true
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "__defrecord"), defRecordVr)
}

// ---- hierarchy.go ----
// hierarchy.go — Clojure hierarchy support for isa?/derive/underive.
//
// A hierarchy is a directed acyclic graph (DAG) of parent-child
// relationships between keywords and symbols. The global hierarchy
// is stored as a var and used by default for isa?/derive/underive.

// Hierarchy represents a Clojure hierarchy.
type Hierarchy struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	mu         sync.RWMutex
	parents    map[string]map[string]bool  // child key → set of parent keys
	parentKeys map[string]coretypes.Object // key → object (for iteration)
	childKeys  map[string]coretypes.Object
}

func MakeHierarchy() *Hierarchy {
	return &Hierarchy{
		parents:    make(map[string]map[string]bool),
		parentKeys: make(map[string]coretypes.Object),
		childKeys:  make(map[string]coretypes.Object),
	}
}

func (h *Hierarchy) ToString(escape bool) string   { return "#object[Hierarchy]" }
func (h *Hierarchy) Equals(other interface{}) bool { return h == other }
func (h *Hierarchy) GetType() *coretypes.Type      { return TYPE.Fn }
func (h *Hierarchy) Hash() uint32                  { return hashutil.Ptr(uintptr(unsafe.Pointer(h))) }
func (h *Hierarchy) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	h.Info = info
	return h
}
func (h *Hierarchy) WithMeta(m coretypes.Map) coretypes.Object {
	h.Meta = coretypes.SafeMerge(h.Meta, m)
	return h
}

func objKey(obj coretypes.Object) string {
	if obj == nil {
		return "nil"
	}
	return obj.GetType().ToString(false) + "|" + obj.ToString(false)
}

// Derive adds a parent relationship: child isa? parent
func (h *Hierarchy) Derive(child, parent coretypes.Object) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ck := objKey(child)
	pk := objKey(parent)

	if h.parents[ck] == nil {
		h.parents[ck] = make(map[string]bool)
	}
	h.parents[ck][pk] = true
	h.parentKeys[pk] = parent
	h.childKeys[ck] = child
}

// Underive removes a parent relationship.
func (h *Hierarchy) Underive(child, parent coretypes.Object) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ck := objKey(child)
	pk := objKey(parent)

	if ps, ok := h.parents[ck]; ok {
		delete(ps, pk)
		if len(ps) == 0 {
			delete(h.parents, ck)
		}
	}
}

// IsA checks if child isa? parent (direct or transitive).
func (h *Hierarchy) IsA(child, parent coretypes.Object) bool {
	if child.Equals(parent) {
		return true
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.isALocked(objKey(child), objKey(parent), make(map[string]bool))
}

func (h *Hierarchy) isALocked(ck, pk string, visited map[string]bool) bool {
	if visited[ck] {
		return false
	}
	visited[ck] = true

	ps, ok := h.parents[ck]
	if !ok {
		return false
	}
	if ps[pk] {
		return true
	}
	// Transitive check
	for parentKey := range ps {
		if h.isALocked(parentKey, pk, visited) {
			return true
		}
	}
	return false
}

// Parents returns direct parents of tag.
func (h *Hierarchy) Parents(tag coretypes.Object) []coretypes.Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	tk := objKey(tag)
	ps, ok := h.parents[tk]
	if !ok {
		return nil
	}
	result := make([]coretypes.Object, 0, len(ps))
	for pk := range ps {
		if obj, ok := h.parentKeys[pk]; ok {
			result = append(result, obj)
		}
	}
	return result
}

// Ancestors returns all transitive ancestors of tag.
func (h *Hierarchy) Ancestors(tag coretypes.Object) []coretypes.Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]coretypes.Object, 0)
	visited := make(map[string]bool)
	h.collectAncestors(objKey(tag), &result, visited)
	return result
}

func (h *Hierarchy) collectAncestors(tk string, result *[]coretypes.Object, visited map[string]bool) {
	ps, ok := h.parents[tk]
	if !ok {
		return
	}
	for pk := range ps {
		if !visited[pk] {
			visited[pk] = true
			if obj, ok := h.parentKeys[pk]; ok {
				*result = append(*result, obj)
			}
			h.collectAncestors(pk, result, visited)
		}
	}
}

// Descendants returns all transitive descendants of tag.
func (h *Hierarchy) Descendants(tag coretypes.Object) []coretypes.Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	pk := objKey(tag)
	result := make([]coretypes.Object, 0)
	visited := make(map[string]bool)

	for ck, ps := range h.parents {
		if ps[pk] && !visited[ck] {
			visited[ck] = true
			if obj, ok := h.childKeys[ck]; ok {
				result = append(result, obj)
			}
			h.collectDescendants(ck, &result, visited)
		}
	}
	return result
}

func (h *Hierarchy) collectDescendants(pk string, result *[]coretypes.Object, visited map[string]bool) {
	for ck, ps := range h.parents {
		if ps[pk] && !visited[ck] {
			visited[ck] = true
			if obj, ok := h.childKeys[ck]; ok {
				*result = append(*result, obj)
			}
			h.collectDescendants(ck, result, visited)
		}
	}
}

// Global hierarchy
var globalHierarchy = MakeHierarchy()

// ---- hierarchy_init.go ----
// hierarchy_init.go — Register derive, underive, isa?, ancestors, descendants, parents, make-hierarchy.

func init() {
	registerHierarchyProcs()
}

func registerHierarchyProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// make-hierarchy
	mhVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "make-hierarchy"))
	mhVr.Value = Proc{Name: "procMakeHierarchy", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 0, 0)
		return MakeHierarchy()
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "make-hierarchy"), mhVr)

	// derive — (derive child parent) or (derive h child parent)
	deriveVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "derive"))
	deriveVr.Value = Proc{Name: "procDerive", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 2:
			globalHierarchy.Derive(args[0], args[1])
			return NIL
		case 3:
			h, ok := args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 3-arity derive must be a hierarchy"))
			}
			h.Derive(args[1], args[2])
			return h
		default:
			PanicArityMinMax(len(args), 2, 3)
			return NIL
		}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "derive"), deriveVr)

	// underive — (underive child parent) or (underive h child parent)
	underiveVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "underive"))
	underiveVr.Value = Proc{Name: "procUnderive", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 2:
			globalHierarchy.Underive(args[0], args[1])
			return NIL
		case 3:
			h, ok := args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 3-arity underive must be a hierarchy"))
			}
			h.Underive(args[1], args[2])
			return h
		default:
			PanicArityMinMax(len(args), 2, 3)
			return NIL
		}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "underive"), underiveVr)

	// isa? — (isa? child parent) or (isa? h child parent)
	isaVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "isa?"))
	isaVr.Value = Proc{Name: "procIsaQ", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 2:
			return coretypes.MakeBoolean(globalHierarchy.IsA(args[0], args[1]))
		case 3:
			h, ok := args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 3-arity isa? must be a hierarchy"))
			}
			return coretypes.MakeBoolean(h.IsA(args[1], args[2]))
		default:
			PanicArityMinMax(len(args), 2, 3)
			return NIL
		}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "isa?"), isaVr)

	// parents — (parents tag) or (parents h tag)
	parentsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "parents"))
	parentsVr.Value = Proc{Name: "procParents", Fn: func(args []coretypes.Object) coretypes.Object {
		var h *Hierarchy
		var tag coretypes.Object
		switch len(args) {
		case 1:
			h = globalHierarchy
			tag = args[0]
		case 2:
			var ok bool
			h, ok = args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 2-arity parents must be a hierarchy"))
			}
			tag = args[1]
		default:
			PanicArityMinMax(len(args), 1, 2)
			return NIL
		}
		ps := h.Parents(tag)
		if len(ps) == 0 {
			return NIL
		}
		s := corecollections.EmptySet()
		for _, p := range ps {
			s = s.Conj(p).(*corecollections.MapSet)
		}
		return s
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "parents"), parentsVr)

	// ancestors — (ancestors tag) or (ancestors h tag)
	ancestorsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "ancestors"))
	ancestorsVr.Value = Proc{Name: "procAncestors", Fn: func(args []coretypes.Object) coretypes.Object {
		var h *Hierarchy
		var tag coretypes.Object
		switch len(args) {
		case 1:
			h = globalHierarchy
			tag = args[0]
		case 2:
			var ok bool
			h, ok = args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 2-arity ancestors must be a hierarchy"))
			}
			tag = args[1]
		default:
			PanicArityMinMax(len(args), 1, 2)
			return NIL
		}
		as := h.Ancestors(tag)
		if len(as) == 0 {
			return NIL
		}
		s := corecollections.EmptySet()
		for _, a := range as {
			s = s.Conj(a).(*corecollections.MapSet)
		}
		return s
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "ancestors"), ancestorsVr)

	// descendants — (descendants tag) or (descendants h tag)
	descendantsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "descendants"))
	descendantsVr.Value = Proc{Name: "procDescendants", Fn: func(args []coretypes.Object) coretypes.Object {
		var h *Hierarchy
		var tag coretypes.Object
		switch len(args) {
		case 1:
			h = globalHierarchy
			tag = args[0]
		case 2:
			var ok bool
			h, ok = args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 2-arity descendants must be a hierarchy"))
			}
			tag = args[1]
		default:
			PanicArityMinMax(len(args), 1, 2)
			return NIL
		}
		ds := h.Descendants(tag)
		if len(ds) == 0 {
			return NIL
		}
		s := corecollections.EmptySet()
		for _, d := range ds {
			s = s.Conj(d).(*corecollections.MapSet)
		}
		return s
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "descendants"), descendantsVr)
}
