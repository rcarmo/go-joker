package core

import "sync"

var transientProcsOnce sync.Once

func init() {
	initTransientProcs()
}

// initTransientProcs registers transient! assoc! conj! persistent! in the core namespace.
func initTransientProcs() {
	transientProcsOnce.Do(func() {
		ns := GLOBAL_ENV.CoreNamespace
		procs := []struct {
			name  string
			fn    func([]Object) Object
			pname string
		}{
			{"transient", procTransient, "procTransient"},
			{"assoc!", procAssocBang, "procAssocBang"},
			{"conj!", procConjBang, "procConjBang"},
			{"persistent!", procPersistentBang, "procPersistentBang"},
		}
		for _, p := range procs {
			sym := MakeSymbol(p.name)
			vr := ns.Intern(sym)
			vr.Value = Proc{Fn: p.fn, Name: p.pname}
			referToUser(sym, vr)
		}

		// transient?
		tqSym := MakeSymbol("transient?")
		tqVr := ns.Intern(tqSym)
		tqVr.Value = Proc{Name: "procTransientQ", Fn: func(args []Object) Object {
			CheckArity(args, 1, 1)
			switch args[0].(type) {
			case *TransientVector:
				return Boolean{B: true}
			}
			return Boolean{B: false}
		}}
		referToUser(tqSym, tqVr)

		// pop! — (pop! tv)
		popSym := MakeSymbol("pop!")
		popVr := ns.Intern(popSym)
		popVr.Value = Proc{Name: "procPopBang", Fn: func(args []Object) Object {
			CheckArity(args, 1, 1)
			if tv, ok := args[0].(*TransientVector); ok {
				if tv.frozen {
					panic(RT.NewError("Cannot mutate a frozen transient"))
				}
				if len(tv.arr) == 0 {
					panic(RT.NewError("Can't pop empty vector"))
				}
				tv.arr = tv.arr[:len(tv.arr)-1]
				return tv
			}
			panic(RT.NewError("pop! requires a transient vector"))
		}}
		referToUser(popSym, popVr)
	})
}
