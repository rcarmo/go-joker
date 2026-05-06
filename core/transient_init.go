package core

import "sync"

var transientProcsOnce sync.Once

// initTransientProcs registers transient! assoc! conj! persistent! in the core namespace.
func initTransientProcs() {
	transientProcsOnce.Do(func() {
		ns := GLOBAL_ENV.CoreNamespace
		curNs := GLOBAL_ENV.CurrentNamespace()
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
			if curNs != nil && curNs != ns {
				curNs.mappings[sym.name] = vr
			}
		}
	})
}
