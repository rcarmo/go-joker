package core

// atom_ext.go — Atom extensions: validators, watches, compare-and-set!

import "sync"

// atomExtras holds validator and watches for an Atom.
// Stored in a side table to avoid changing the Atom struct.
type atomExtras struct {
	validator Callable
	watches   map[string]struct {
		key Object
		fn  Callable
	} // key.ToString → watch
}

var atomExtrasMap sync.Map // *Atom → *atomExtras

func getAtomExtras(a *Atom) *atomExtras {
	if v, ok := atomExtrasMap.Load(a); ok {
		return v.(*atomExtras)
	}
	return nil
}

func getOrCreateAtomExtras(a *Atom) *atomExtras {
	if v, ok := atomExtrasMap.Load(a); ok {
		return v.(*atomExtras)
	}
	ext := &atomExtras{watches: make(map[string]struct {
		key Object
		fn  Callable
	})}
	atomExtrasMap.Store(a, ext)
	return ext
}

// notifyWatches calls all watch functions with (key atom old-val new-val).
func notifyWatches(a *Atom, oldVal, newVal Object) {
	ext := getAtomExtras(a)
	if ext == nil || len(ext.watches) == 0 {
		return
	}
	for _, w := range ext.watches {
		call4(w.fn, w.key, a, oldVal, newVal)
	}
}

// validateAtom checks the validator, panics if invalid.
func validateAtom(a *Atom, newVal Object) {
	ext := getAtomExtras(a)
	if ext == nil || ext.validator == nil {
		return
	}
	result := call1(ext.validator, newVal)
	if !ToBool(result) {
		panic(RT.NewError("Invalid reference state"))
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
	svVr := ns.Intern(MakeSymbol("set-validator!"))
	svVr.Value = Proc{Name: "procSetValidator", Fn: func(args []Object) Object {
		CheckArity(args, 2, 2)
		a := EnsureObjectIsAtom(args[0], "set-validator! requires an atom, got %s")
		ext := getOrCreateAtomExtras(a)
		if args[1] == nil || IsNil(args[1]) {
			ext.validator = nil
		} else {
			fn := EnsureObjectIsCallable(args[1], "validator must be a function, got %s")
			// Validate current value
			result := call1(fn, a.Deref())
			if !ToBool(result) {
				panic(RT.NewError("Invalid reference state"))
			}
			ext.validator = fn
		}
		return NIL
	}}
	referToUser(MakeSymbol("set-validator!"), svVr)

	// get-validator — (get-validator atom)
	gvVr := ns.Intern(MakeSymbol("get-validator"))
	gvVr.Value = Proc{Name: "procGetValidator", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		a := EnsureObjectIsAtom(args[0], "get-validator requires an atom, got %s")
		ext := getAtomExtras(a)
		if ext == nil || ext.validator == nil {
			return NIL
		}
		return ext.validator.(Object)
	}}
	referToUser(MakeSymbol("get-validator"), gvVr)

	// add-watch — (add-watch atom key fn)
	awVr := ns.Intern(MakeSymbol("add-watch"))
	awVr.Value = Proc{Name: "procAddWatch", Fn: func(args []Object) Object {
		CheckArity(args, 3, 3)
		a := EnsureObjectIsAtom(args[0], "add-watch requires an atom, got %s")
		key := args[1]
		fn := EnsureObjectIsCallable(args[2], "watch function must be callable, got %s")
		ext := getOrCreateAtomExtras(a)
		ext.watches[key.ToString(false)] = struct {
			key Object
			fn  Callable
		}{key, fn}
		return a
	}}
	referToUser(MakeSymbol("add-watch"), awVr)

	// remove-watch — (remove-watch atom key)
	rwVr := ns.Intern(MakeSymbol("remove-watch"))
	rwVr.Value = Proc{Name: "procRemoveWatch", Fn: func(args []Object) Object {
		CheckArity(args, 2, 2)
		a := EnsureObjectIsAtom(args[0], "remove-watch requires an atom, got %s")
		key := args[1]
		ext := getAtomExtras(a)
		if ext != nil {
			delete(ext.watches, key.ToString(false))
		}
		return a
	}}
	referToUser(MakeSymbol("remove-watch"), rwVr)

	// compare-and-set! — (compare-and-set! atom oldval newval)
	casVr := ns.Intern(MakeSymbol("compare-and-set!"))
	casVr.Value = Proc{Name: "procCompareAndSet", Fn: func(args []Object) Object {
		CheckArity(args, 3, 3)
		a := EnsureObjectIsAtom(args[0], "compare-and-set! requires an atom, got %s")
		oldVal := args[1]
		newVal := args[2]
		current := a.Deref()
		if current.Equals(oldVal) {
			validateAtom(a, newVal)
			old := a.value
			a.value = newVal
			notifyWatches(a, old, newVal)
			return Boolean{B: true}
		}
		return Boolean{B: false}
	}}
	referToUser(MakeSymbol("compare-and-set!"), casVr)
}

// IsNil checks if an Object is nil or Nil.
func IsNil(obj Object) bool {
	if obj == nil {
		return true
	}
	_, ok := obj.(Nil)
	return ok
}
