package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"sync"
)

// ---- string_cursor_procs.go ----

var stringCursorInitOnce sync.Once

func initStringCursorProcs() {
	stringCursorInitOnce.Do(func() {
		ns := GLOBAL_ENV.CoreNamespace
		procs := []struct {
			name  string
			fn    func([]coretypes.Object) coretypes.Object
			pname string
		}{
			{"string-cursor", procStringCursor, "procStringCursor"},
			{"cursor-char", procCursorChar, "procCursorChar"},
			{"cursor-next", procCursorNext, "procCursorNext"},
			{"cursor-done?", procCursorDone, "procCursorDone"},
			{"cursor-index", procCursorIndex, "procCursorIndex"},
		}
		for _, p := range procs {
			sym := coretypes.MakeSymbol(STRINGS.Intern, p.name)
			vr := ns.Intern(sym)
			vr.Value = Proc{Fn: p.fn, Name: p.pname}
			curNs := GLOBAL_ENV.CurrentNamespace()
			if curNs != nil && curNs != ns {
				curNs.mappings[sym.NameKey()] = vr
			}
		}
	})
}

func procStringCursor(args []coretypes.Object) coretypes.Object {
	s, ok := args[0].(coretypes.String)
	if !ok {
		panic(RT.NewError("string-cursor expects a string argument"))
	}
	return NewStringCursor(s.S)
}

func procCursorChar(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-char expects a StringCursor"))
	}
	r := c.Char()
	if r < 0 {
		return NIL
	}
	return coretypes.Char{Ch: r}
}

func procCursorNext(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-next expects a StringCursor"))
	}
	return c.Next()
}

func procCursorDone(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-done? expects a StringCursor"))
	}
	return coretypes.Boolean{B: c.Done()}
}

func procCursorIndex(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-index expects a StringCursor"))
	}
	return coretypes.Int{I: c.Index()}
}
