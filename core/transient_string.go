package core

import "os"

// transient_string.go — internal mutable string builder for IR loops.
//
// This is not a user-facing transient. The IR can convert proven-local String
// loop slots to TransientString so repeated `(str s char)` appends mutate a
// byte buffer instead of allocating a fresh String every iteration.

func irStringBuilderEnabled() bool { return os.Getenv("JOKER_IR_STRING_BUILDER") == "1" }

type TransientString struct {
	buf    []byte
	frozen bool
}

func (ts *TransientString) ToString(escape bool) string { return string(ts.buf) }
func (ts *TransientString) Equals(other interface{}) bool {
	switch v := other.(type) {
	case *TransientString:
		return string(ts.buf) == string(v.buf)
	case String:
		return string(ts.buf) == v.S
	default:
		return false
	}
}
func (ts *TransientString) GetInfo() *ObjectInfo        { return nil }
func (ts *TransientString) WithInfo(*ObjectInfo) Object { return ts }
func (ts *TransientString) GetType() *Type              { return TYPE.String }
func (ts *TransientString) Hash() uint32                { return String{S: string(ts.buf)}.Hash() }
func (ts *TransientString) Count() int                  { return stringRuneCountFastCompat(string(ts.buf)) }

func stringRuneCountFastCompat(s string) int {
	// Avoid importing utf8 in this tiny helper file; String.Count has the fully
	// correct implementation. ASCII is the hot path for the builder.
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return String{S: s}.Count()
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

func (ts *TransientString) ToPersistent() String {
	ts.frozen = true
	return String{S: string(ts.buf)}
}

func ToTransientString(s String) *TransientString {
	buf := make([]byte, len(s.S), len(s.S)+16)
	copy(buf, s.S)
	return &TransientString{buf: buf}
}
