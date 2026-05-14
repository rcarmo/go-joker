package core

import corecursor "github.com/rcarmo/go-joker/core/cursor"

// StringCursor wraps the extracted string cursor implementation with core's
// Object protocol. Runtime cursor mechanics live in core/cursor; this file is
// only the root object/protocol adapter required by existing Joker objects.
type StringCursor struct {
	InfoHolder
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

func (c *StringCursor) GetInfo() *ObjectInfo { return nil }

func (c *StringCursor) Hash() uint32 { return c.cur.Hash() }

func (c *StringCursor) WithInfo(info *ObjectInfo) Object { return c }

func (c *StringCursor) GetType() *Type { return typeStringCursor }

var typeStringCursor = &Type{name: "StringCursor"}
