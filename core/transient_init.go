package core

import "sync"

var transientProcsOnce sync.Once

func init() {
	initTransientProcs()
}

// initTransientProcs registers transient, assoc!, conj!, persistent!, transient?, and pop! in the core namespace.
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
		tqVr.Value = Proc{Name: "procTransientQ", Fn: procIsTransient}
		referToUser(tqSym, tqVr)

		// pop! — (pop! tv)
		popSym := MakeSymbol("pop!")
		popVr := ns.Intern(popSym)
		popVr.Value = Proc{Name: "procPopBang", Fn: procPopBang}
		referToUser(popSym, popVr)
	})
}
