package core

// atom_ext.go — Atom extensions: validators, watches, compare-and-set!

import (
	"sync"

	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

// atomExtras holds validator and watches for an Atom.
// Stored in a side table to avoid changing the Atom struct.
type atomExtras struct {
	validator coretypes.Callable
	watches   map[string]struct {
		key coretypes.Object
		fn  coretypes.Callable
	} // key.ToString → watch
}

var atomExtrasMap sync.Map // *corert.Atom → *atomExtras

func getAtomExtras(a *corert.Atom) *atomExtras {
	if v, ok := atomExtrasMap.Load(a); ok {
		return v.(*atomExtras)
	}
	return nil
}

func getOrCreateAtomExtras(a *corert.Atom) *atomExtras {
	if v, ok := atomExtrasMap.Load(a); ok {
		return v.(*atomExtras)
	}
	ext := &atomExtras{watches: make(map[string]struct {
		key coretypes.Object
		fn  coretypes.Callable
	})}
	atomExtrasMap.Store(a, ext)
	return ext
}

// notifyWatches calls all watch functions with (key atom old-val new-val).
func notifyWatches(a *corert.Atom, oldVal, newVal coretypes.Object) {
	ext := getAtomExtras(a)
	if ext == nil || len(ext.watches) == 0 {
		return
	}
	for _, w := range ext.watches {
		call4(w.fn, w.key, a, oldVal, newVal)
	}
}

// validateAtom checks the validator, panics if invalid.
func validateAtom(a *corert.Atom, newVal coretypes.Object) {
	ext := getAtomExtras(a)
	if ext == nil || ext.validator == nil {
		return
	}
	result := call1(ext.validator, newVal)
	if !ToBool(result) {
		panic(coretypes.RuntimeError("Invalid reference state"))
	}
}

func init() {
	registerAtomExtProcs()
}

func registerAtomExtProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// set-validator! — (set-validator! atom fn)
	svVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "set-validator!"))
	svVr.Value = Proc{Name: "procSetValidator", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		a := EnsureObjectIsAtom(args[0], "set-validator! requires an atom, got %s")
		ext := getOrCreateAtomExtras(a)
		if args[1] == nil || IsNil(args[1]) {
			ext.validator = nil
		} else {
			fn := coretypes.EnsureObjectIsCallable(args[1], "validator must be a function, got %s")
			// Validate current value
			result := call1(fn, a.Deref())
			if !ToBool(result) {
				panic(coretypes.RuntimeError("Invalid reference state"))
			}
			ext.validator = fn
		}
		return NIL
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "set-validator!"), svVr)

	// get-validator — (get-validator atom)
	gvVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "get-validator"))
	gvVr.Value = Proc{Name: "procGetValidator", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		a := EnsureObjectIsAtom(args[0], "get-validator requires an atom, got %s")
		ext := getAtomExtras(a)
		if ext == nil || ext.validator == nil {
			return NIL
		}
		return ext.validator.(coretypes.Object)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "get-validator"), gvVr)

	// add-watch — (add-watch atom key fn)
	awVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "add-watch"))
	awVr.Value = Proc{Name: "procAddWatch", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 3, 3)
		a := EnsureObjectIsAtom(args[0], "add-watch requires an atom, got %s")
		key := args[1]
		fn := coretypes.EnsureObjectIsCallable(args[2], "watch function must be callable, got %s")
		ext := getOrCreateAtomExtras(a)
		ext.watches[key.ToString(false)] = struct {
			key coretypes.Object
			fn  coretypes.Callable
		}{key, fn}
		return a
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "add-watch"), awVr)

	// remove-watch — (remove-watch atom key)
	rwVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "remove-watch"))
	rwVr.Value = Proc{Name: "procRemoveWatch", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		a := EnsureObjectIsAtom(args[0], "remove-watch requires an atom, got %s")
		key := args[1]
		ext := getAtomExtras(a)
		if ext != nil {
			delete(ext.watches, key.ToString(false))
		}
		return a
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "remove-watch"), rwVr)

	// compare-and-set! — (compare-and-set! atom oldval newval)
	casVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "compare-and-set!"))
	casVr.Value = Proc{Name: "procCompareAndSet", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 3, 3)
		a := EnsureObjectIsAtom(args[0], "compare-and-set! requires an atom, got %s")
		oldVal := args[1]
		newVal := args[2]
		old, ok := a.CompareAndSet(oldVal, newVal, func(v coretypes.Object) { validateAtom(a, v) })
		if ok {
			notifyWatches(a, old, newVal)
		}
		return coretypes.Boolean{B: ok}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "compare-and-set!"), casVr)
}

// IsNil checks if an coretypes.Object is nil or Nil.
func IsNil(obj coretypes.Object) bool {
	if obj == nil {
		return true
	}
	_, ok := obj.(Nil)
	return ok
}
