package core

import coretypes "github.com/rcarmo/go-joker/core/types"

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

func installMacro(ns *Namespace, name string, fn func([]Object) Object) {
	sym := MakeSymbol(name)
	vr := ns.Intern(sym)
	vr.Value = Proc{Name: "macro" + name, Fn: fn}
	vr.isMacro = true
	referToUser(sym, vr)
}

func listObjs(objs ...Object) *List { return collectionConstruction.NewListFrom(objs...) }
func quoteObj(obj Object) *List     { return listObjs(MakeSymbol("quote"), obj) }
func doObj(forms ...Object) *List {
	return collectionConstruction.NewListFrom(append([]Object{MakeSymbol("do")}, forms...)...)
}

func macroDefProtocol(args []Object) Object {
	// macro args: &form, &env, name, method...
	if len(args) < 3 {
		panic(RT.NewError("defprotocol requires a name"))
	}
	name, ok := args[2].(Symbol)
	if !ok {
		panic(RT.NewError("defprotocol name must be a symbol"))
	}
	forms := []Object{MakeSymbol("__defprotocol"), quoteObj(name)}
	for _, raw := range args[3:] {
		seqable, ok := raw.(Seqable)
		if !ok {
			continue // docstrings/options are ignored by the compact runtime protocol helper
		}
		s := seqable.Seq()
		if s.IsEmpty() {
			continue
		}
		mname, ok := s.First().(Symbol)
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
		forms = append(forms, String{S: mname.ToString(false)}, Int{I: argv.Count()})
	}
	return collectionConstruction.NewListFrom(forms...)
}

func macroExtendType(args []Object) Object {
	// (extend-type Type Proto (method [args] body...) Proto2 ...)
	if len(args) < 5 {
		panic(RT.NewError("extend-type requires a type, protocol, and method implementations"))
	}
	typeName := macroTypeName(args[2])
	forms := make([]Object, 0)
	i := 3
	for i < len(args) {
		proto := args[i]
		i++
		call := []Object{MakeSymbol("__extend-type"), proto, String{S: typeName}}
		for i < len(args) {
			if _, isProto := args[i].(Symbol); isProto && i+1 < len(args) {
				if _, nextIsMethod := args[i+1].(Seqable); nextIsMethod {
					break
				}
			}
			method, ok := args[i].(Seqable)
			if !ok {
				break
			}
			s := method.Seq()
			if s.IsEmpty() {
				i++
				continue
			}
			mname, ok := s.First().(Symbol)
			if !ok {
				i++
				continue
			}
			fnTail := ToSlice(s.Rest())
			fnForm := collectionConstruction.NewListFrom(append([]Object{MakeSymbol("fn")}, fnTail...)...)
			call = append(call, String{S: mname.ToString(false)}, fnForm)
			i++
		}
		forms = append(forms, collectionConstruction.NewListFrom(call...))
	}
	return doObj(forms...)
}

func macroExtendProtocol(args []Object) Object {
	// (extend-protocol Proto Type (method [args] body...) Type2 ...)
	if len(args) < 5 {
		panic(RT.NewError("extend-protocol requires a protocol, type, and method implementations"))
	}
	proto := args[2]
	forms := make([]Object, 0)
	i := 3
	for i < len(args) {
		typeName := macroTypeName(args[i])
		i++
		call := []Object{MakeSymbol("__extend-type"), proto, String{S: typeName}}
		for i < len(args) {
			method, ok := args[i].(Seqable)
			if !ok {
				break
			}
			s := method.Seq()
			if s.IsEmpty() {
				i++
				continue
			}
			mname, ok := s.First().(Symbol)
			if !ok {
				i++
				continue
			}
			fnTail := ToSlice(s.Rest())
			fnForm := collectionConstruction.NewListFrom(append([]Object{MakeSymbol("fn")}, fnTail...)...)
			call = append(call, String{S: mname.ToString(false)}, fnForm)
			i++
			// Stop if the next form looks like a type followed by methods. In practice
			// a new type is a symbol/string/keyword and a method implementation is a list.
			if i < len(args) {
				if _, ok := args[i].(Seqable); !ok {
					break
				}
			}
		}
		forms = append(forms, collectionConstruction.NewListFrom(call...))
	}
	return doObj(forms...)
}

func macroDefRecord(args []Object) Object {
	// (defrecord Name [fields] Protocol (method [args] body...) ...)
	if len(args) < 4 {
		panic(RT.NewError("defrecord requires a name and fields vector"))
	}
	name, ok := args[2].(Symbol)
	if !ok {
		panic(RT.NewError("defrecord name must be a symbol"))
	}
	fieldsSeq, ok := args[3].(Seqable)
	if !ok {
		panic(RT.NewError("defrecord fields must be seqable"))
	}
	defCall := []Object{MakeSymbol("__defrecord"), quoteObj(name)}
	for s := fieldsSeq.Seq(); !s.IsEmpty(); s = s.Rest() {
		field, ok := s.First().(Symbol)
		if !ok {
			panic(RT.NewError("defrecord field must be a symbol"))
		}
		defCall = append(defCall, String{S: field.ToString(false)})
	}
	forms := []Object{collectionConstruction.NewListFrom(defCall...)}
	if len(args) > 4 {
		// Reuse extend-type semantics with the record's dispatch type name.
		extendArgs := append([]Object{args[0], args[1], name}, args[4:]...)
		forms = append(forms, macroExtendType(extendArgs))
	}
	return doObj(forms...)
}

func macroTypeName(obj Object) string {
	switch t := obj.(type) {
	case Symbol:
		return t.ToString(false)
	case String:
		return t.S
	case Keyword:
		return t.ToString(false)[1:]
	default:
		return obj.ToString(false)
	}
}
