package core

import (
	corecursor "github.com/rcarmo/go-joker/core/cursor"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"sync"
)

// ---- string_cursor.go ----
// StringCursor wraps the extracted string cursor implementation with core's
// coretypes.Object protocol. Runtime cursor mechanics live in core/cursor; this file is
// only the root object/protocol adapter required by existing Joker objects.
type StringCursor struct {
	coretypes.InfoHolder
	cur *corecursor.Cursor
}

func NewStringCursor(s string) *StringCursor {
	return &StringCursor{cur: corecursor.New(s)}
}

func (c *StringCursor) Done() bool { return c.cur.Done() }

func (c *StringCursor) Char() rune { return c.cur.Char() }

func (c *StringCursor) Next() *StringCursor {
	next := c.cur.Next()
	if next == c.cur {
		return c
	}
	return &StringCursor{cur: next}
}

func (c *StringCursor) Index() int { return c.cur.Index() }

// --- coretypes.Object interface ---

func (c *StringCursor) ToString(escape bool) string { return c.cur.String() }

func (c *StringCursor) Equals(other interface{}) bool {
	if o, ok := other.(*StringCursor); ok {
		return c.cur.Equal(o.cur)
	}
	return false
}

func (c *StringCursor) GetInfo() *coretypes.ObjectInfo { return nil }

func (c *StringCursor) Hash() uint32 { return c.cur.Hash() }

func (c *StringCursor) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return c }

func (c *StringCursor) GetType() *coretypes.Type { return typeStringCursor }

var typeStringCursor = &coretypes.Type{Name: "StringCursor"}

// ---- string_cursor_procs.go ----
// String cursor procs — registered in procs_slow_init.go or inline

var stringCursorInitOnce sync.Once

// initStringCursorProcs must be called after GLOBAL_ENV is initialized.
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
			// Also refer in current namespace so symbol resolution works
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
