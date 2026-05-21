package types

import (
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

type StringCursor struct {
	InfoHolder
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
func (c *StringCursor) GetInfo() *ObjectInfo             { return nil }
func (c *StringCursor) Hash() uint32                     { return c.rt.Hash() }
func (c *StringCursor) WithInfo(info *ObjectInfo) Object { return c }
func (c *StringCursor) GetType() *Type                   { return TypeStringCursor }

var TypeStringCursor = &Type{Name: "StringCursor"}

type TransientString struct {
	rt *corestr.RuntimeTransientString
}

func NewTransientString(s String) Object {
	return &TransientString{rt: corestr.NewRuntimeTransientString(s.S)}
}

func (ts *TransientString) ToString(escape bool) string { return ts.rt.String() }
func (ts *TransientString) Equals(other interface{}) bool {
	switch v := other.(type) {
	case *TransientString:
		return ts.rt.String() == v.rt.String()
	case String:
		return ts.rt.String() == v.S
	default:
		return false
	}
}
func (ts *TransientString) GetInfo() *ObjectInfo                   { return nil }
func (ts *TransientString) WithInfo(*ObjectInfo) Object            { return ts }
func (ts *TransientString) GetType() *Type                         { return RuntimeTypes.String }
func (ts *TransientString) Hash() uint32                           { return String{S: ts.rt.String()}.Hash() }
func (ts *TransientString) Count() int                             { return ts.rt.Count() }
func (ts *TransientString) AppendChar(ch rune) *TransientString    { ts.rt.AppendChar(ch); return ts }
func (ts *TransientString) AppendString(s string) *TransientString { ts.rt.AppendString(s); return ts }
func (ts *TransientString) PrependChar(ch rune) *TransientString   { ts.rt.PrependChar(ch); return ts }
func (ts *TransientString) PrependString(s string) *TransientString {
	ts.rt.PrependString(s)
	return ts
}
func (ts *TransientString) ToPersistent() String {
	return String{S: ts.rt.Freeze()}
}
