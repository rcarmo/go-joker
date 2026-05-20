package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corestr "github.com/rcarmo/go-joker/core/types/string"
	"sync"
)

type StringCursor struct {
	coretypes.InfoHolder
	rt *corestr.CursorRuntime
}

func NewStringCursor(s string) *StringCursor { return &StringCursor{rt: corestr.NewCursorRuntime(s)} }
func (c *StringCursor) Done() bool           { return c.rt.Done() }
func (c *StringCursor) Char() rune           { return c.rt.Char() }
func (c *StringCursor) Index() int           { return c.rt.Index() }
func (c *StringCursor) Next() *StringCursor {
	next := c.rt.Next()
	if next == c.rt {
		return c
	}
	return &StringCursor{rt: next}
}
func (c *StringCursor) ToString(escape bool) string { return c.rt.String() }
func (c *StringCursor) Equals(other interface{}) bool {
	o, ok := other.(*StringCursor)
	return ok && c.rt.Equal(o.rt)
}
func (c *StringCursor) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (c *StringCursor) Hash() uint32                                         { return c.rt.Hash() }
func (c *StringCursor) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return c }
func (c *StringCursor) GetType() *coretypes.Type                             { return typeStringCursor }

var typeStringCursor = &coretypes.Type{Name: "StringCursor"}

type TransientString struct {
	rt *corestr.RuntimeTransientString
}

func NewTransientString(s coretypes.String) coretypes.Object {
	return &TransientString{rt: corestr.NewRuntimeTransientString(s.S)}
}

func (ts *TransientString) ToString(escape bool) string { return ts.rt.String() }
func (ts *TransientString) Equals(other interface{}) bool {
	switch v := other.(type) {
	case *TransientString:
		return ts.rt.String() == v.rt.String()
	case coretypes.String:
		return ts.rt.String() == v.S
	default:
		return false
	}
}
func (ts *TransientString) GetInfo() *coretypes.ObjectInfo                  { return nil }
func (ts *TransientString) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return ts }
func (ts *TransientString) GetType() *coretypes.Type                        { return TYPE.String }
func (ts *TransientString) Hash() uint32                                    { return coretypes.String{S: ts.rt.String()}.Hash() }
func (ts *TransientString) Count() int                                      { return ts.rt.Count() }
func (ts *TransientString) AppendChar(ch rune) *TransientString             { ts.rt.AppendChar(ch); return ts }
func (ts *TransientString) AppendString(s string) *TransientString          { ts.rt.AppendString(s); return ts }
func (ts *TransientString) PrependChar(ch rune) *TransientString            { ts.rt.PrependChar(ch); return ts }
func (ts *TransientString) PrependString(s string) *TransientString {
	ts.rt.PrependString(s)
	return ts
}
func (ts *TransientString) ToPersistent() coretypes.String {
	return coretypes.String{S: ts.rt.Freeze()}
}

// ---- string_cursor.go ----
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
