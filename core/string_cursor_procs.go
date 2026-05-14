package core

import "sync"

// String cursor procs — registered in procs_slow_init.go or inline

var stringCursorInitOnce sync.Once

// initStringCursorProcs must be called after GLOBAL_ENV is initialized.
func initStringCursorProcs() {
	stringCursorInitOnce.Do(func() {
		ns := GLOBAL_ENV.CoreNamespace
		procs := []struct {
			name  string
			fn    func([]Object) Object
			pname string
		}{
			{"string-cursor", procStringCursor, "procStringCursor"},
			{"cursor-char", procCursorChar, "procCursorChar"},
			{"cursor-next", procCursorNext, "procCursorNext"},
			{"cursor-done?", procCursorDone, "procCursorDone"},
			{"cursor-index", procCursorIndex, "procCursorIndex"},
		}
		for _, p := range procs {
			sym := MakeSymbol(p.name)
			vr := ns.Intern(sym)
			vr.Value = Proc{Fn: p.fn, Name: p.pname}
			// Also refer in current namespace so symbol resolution works
			curNs := GLOBAL_ENV.CurrentNamespace()
			if curNs != nil && curNs != ns {
				curNs.mappings[sym.name] = vr
			}
		}
	})
}

func procStringCursor(args []Object) Object {
	s, ok := args[0].(String)
	if !ok {
		panic(RT.NewError("string-cursor expects a string argument"))
	}
	return NewStringCursor(s.S)
}

func procCursorChar(args []Object) Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-char expects a StringCursor"))
	}
	r := c.Char()
	if r < 0 {
		return NIL
	}
	return Char{Ch: r}
}

func procCursorNext(args []Object) Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-next expects a StringCursor"))
	}
	return c.Next()
}

func procCursorDone(args []Object) Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-done? expects a StringCursor"))
	}
	return Boolean{B: c.Done()}
}

func procCursorIndex(args []Object) Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-index expects a StringCursor"))
	}
	return Int{I: c.Index()}
}
