package core

import coretypes "github.com/rcarmo/go-joker/core/types"

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
