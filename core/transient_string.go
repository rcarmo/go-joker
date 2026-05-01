package core

import "os"

// transient_string.go — internal mutable string builder for IR loops.
//
// This is not a user-facing transient. The IR can convert proven-local String
// loop slots to TransientString so repeated `(str s char)` appends mutate a
// byte buffer instead of allocating a fresh String every iteration.

func irStringBuilderMode() string {
	mode := os.Getenv("JOKER_IR_STRING_BUILDER")
	if mode == "" {
		return "auto"
	}
	return mode
}
func irStringBuilderForce() bool {
	mode := irStringBuilderMode()
	return mode == "1" || mode == "force" || mode == "all"
}
func irStringBuilderDisabled() bool {
	mode := irStringBuilderMode()
	return mode == "0" || mode == "off" || mode == "false"
}

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

func (ts *TransientString) ToPersistent() String {
	ts.frozen = true
	return String{S: string(ts.buf)}
}

func ToTransientString(s String) *TransientString {
	buf := make([]byte, len(s.S), len(s.S)+16)
	copy(buf, s.S)
	return &TransientString{buf: buf}
}
