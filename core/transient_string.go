package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

// transient_string.go — internal mutable string builder for IR loops.

type TransientString struct {
	b *corestr.TransientBuilder
}

func (ts *TransientString) ToString(escape bool) string { return ts.b.String() }
func (ts *TransientString) Equals(other interface{}) bool {
	switch v := other.(type) {
	case *TransientString:
		return ts.b.String() == v.b.String()
	case coretypes.String:
		return ts.b.String() == v.S
	default:
		return false
	}
}
func (ts *TransientString) GetInfo() *coretypes.ObjectInfo                  { return nil }
func (ts *TransientString) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return ts }
func (ts *TransientString) GetType() *coretypes.Type                        { return TYPE.String }
func (ts *TransientString) Hash() uint32                                    { return coretypes.String{S: ts.b.String()}.Hash() }
func (ts *TransientString) Count() int                                      { return ts.b.Count() }

func (ts *TransientString) AppendChar(ch rune) *TransientString {
	ts.b.AppendChar(ch)
	return ts
}

func (ts *TransientString) AppendString(s string) *TransientString {
	ts.b.AppendString(s)
	return ts
}

func (ts *TransientString) PrependChar(ch rune) *TransientString {
	ts.b.PrependChar(ch)
	return ts
}

func (ts *TransientString) PrependString(s string) *TransientString {
	ts.b.PrependString(s)
	return ts
}

func (ts *TransientString) ToPersistent() coretypes.String {
	return coretypes.String{S: ts.b.Freeze()}
}

func ToTransientString(s coretypes.String) *TransientString {
	return &TransientString{b: corestr.NewTransientBuilder(s.S, 16)}
}
