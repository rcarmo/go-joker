package core

import (
	corecursor "github.com/rcarmo/go-joker/core/cursor"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"sync"
)

// ---- string_cursor.go ----
// StringCursor wraps the extracted string cursor implementation with core's
// Object protocol. Runtime cursor mechanics live in core/cursor; this file is
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

// --- Object interface ---

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
	s, ok := args[0].(coretypes.String)
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
	return coretypes.Char{Ch: r}
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
	return coretypes.Boolean{B: c.Done()}
}

func procCursorIndex(args []Object) Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-index expects a StringCursor"))
	}
	return coretypes.Int{I: c.Index()}
}

// ---- transient_string.go ----
// transient_string.go — internal mutable string builder for IR loops.
//
// This is not a user-facing transient. The IR can convert proven-local coretypes.String
// loop slots to TransientString so repeated `(str s char)` appends mutate a
// byte buffer instead of allocating a fresh String every iteration.

type TransientString struct {
	buf    []byte
	frozen bool
}

func (ts *TransientString) ToString(escape bool) string { return string(ts.buf) }
func (ts *TransientString) Equals(other interface{}) bool {
	switch v := other.(type) {
	case *TransientString:
		return string(ts.buf) == string(v.buf)
	case coretypes.String:
		return string(ts.buf) == v.S
	default:
		return false
	}
}
func (ts *TransientString) GetInfo() *coretypes.ObjectInfo        { return nil }
func (ts *TransientString) WithInfo(*coretypes.ObjectInfo) Object { return ts }
func (ts *TransientString) GetType() *coretypes.Type              { return TYPE.String }
func (ts *TransientString) Hash() uint32                          { return coretypes.String{S: string(ts.buf)}.Hash() }
func (ts *TransientString) Count() int                            { return stringRuneCountFastCompat(string(ts.buf)) }

func stringRuneCountFastCompat(s string) int {
	// Avoid importing utf8 in this tiny helper file; String.Count has the fully
	// correct implementation. ASCII is the hot path for the builder.
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return coretypes.String{S: s}.Count()
		}
	}
	return len(s)
}

func (ts *TransientString) checkFrozen() {
	if ts.frozen {
		panic(RT.NewError("Cannot mutate a frozen transient string"))
	}
}

func (ts *TransientString) AppendChar(ch rune) *TransientString {
	ts.checkFrozen()
	if ch >= 0 && ch < 128 {
		ts.buf = append(ts.buf, byte(ch))
	} else {
		ts.buf = append(ts.buf, string(ch)...)
	}
	return ts
}

func (ts *TransientString) AppendString(s string) *TransientString {
	ts.checkFrozen()
	ts.buf = append(ts.buf, s...)
	return ts
}

func (ts *TransientString) PrependChar(ch rune) *TransientString {
	ts.checkFrozen()
	var prefix []byte
	if ch >= 0 && ch < 128 {
		prefix = []byte{byte(ch)}
	} else {
		prefix = []byte(string(ch))
	}
	ts.buf = append(ts.buf, make([]byte, len(prefix))...)
	copy(ts.buf[len(prefix):], ts.buf[:len(ts.buf)-len(prefix)])
	copy(ts.buf, prefix)
	return ts
}

func (ts *TransientString) PrependString(s string) *TransientString {
	ts.checkFrozen()
	if s == "" {
		return ts
	}
	ts.buf = append(ts.buf, make([]byte, len(s))...)
	copy(ts.buf[len(s):], ts.buf[:len(ts.buf)-len(s)])
	copy(ts.buf, s)
	return ts
}

func (ts *TransientString) ToPersistent() coretypes.String {
	ts.frozen = true
	return coretypes.String{S: string(ts.buf)}
}

func ToTransientString(s coretypes.String) *TransientString {
	buf := make([]byte, len(s.S), len(s.S)+16)
	copy(buf, s.S)
	return &TransientString{buf: buf}
}
