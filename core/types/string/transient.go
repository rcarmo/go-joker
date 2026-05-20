package string

import "unicode/utf8"

// TransientBuilder is a mutable string builder used by runtime hot paths.
type TransientBuilder struct {
	buf    []byte
	frozen bool
}

func NewTransientBuilder(s string, extraCap int) *TransientBuilder {
	capHint := len(s) + extraCap
	if capHint < len(s) {
		capHint = len(s)
	}
	buf := make([]byte, len(s), capHint)
	copy(buf, s)
	return &TransientBuilder{buf: buf}
}

func (b *TransientBuilder) String() string { return string(b.buf) }
func (b *TransientBuilder) Count() int {
	if utf8.ValidString(string(b.buf)) {
		return utf8.RuneCount(b.buf)
	}
	return len(b.buf)
}
func (b *TransientBuilder) checkFrozen() {
	if b.frozen {
		panic("cannot mutate a frozen transient string")
	}
}
func (b *TransientBuilder) AppendChar(ch rune) {
	b.checkFrozen()
	if ch >= 0 && ch < 128 {
		b.buf = append(b.buf, byte(ch))
	} else {
		b.buf = append(b.buf, string(ch)...)
	}
}
func (b *TransientBuilder) AppendString(s string) { b.checkFrozen(); b.buf = append(b.buf, s...) }
func (b *TransientBuilder) PrependChar(ch rune) {
	b.checkFrozen()
	var prefix []byte
	if ch >= 0 && ch < 128 {
		prefix = []byte{byte(ch)}
	} else {
		prefix = []byte(string(ch))
	}
	b.buf = append(b.buf, make([]byte, len(prefix))...)
	copy(b.buf[len(prefix):], b.buf[:len(b.buf)-len(prefix)])
	copy(b.buf, prefix)
}
func (b *TransientBuilder) PrependString(s string) {
	b.checkFrozen()
	if s == "" {
		return
	}
	b.buf = append(b.buf, make([]byte, len(s))...)
	copy(b.buf[len(s):], b.buf[:len(b.buf)-len(s)])
	copy(b.buf, s)
}
func (b *TransientBuilder) Freeze() string { b.frozen = true; return string(b.buf) }

// RuntimeTransientString is an internal mutable string builder for IR loops.
type RuntimeTransientString struct{ b *TransientBuilder }

func NewRuntimeTransientString(s string) *RuntimeTransientString {
	return &RuntimeTransientString{b: NewTransientBuilder(s, 16)}
}
func (ts *RuntimeTransientString) String() string { return ts.b.String() }
func (ts *RuntimeTransientString) Count() int     { return ts.b.Count() }
func (ts *RuntimeTransientString) Freeze() string { return ts.b.Freeze() }
func (ts *RuntimeTransientString) AppendChar(ch rune) *RuntimeTransientString {
	ts.b.AppendChar(ch)
	return ts
}
func (ts *RuntimeTransientString) AppendString(s string) *RuntimeTransientString {
	ts.b.AppendString(s)
	return ts
}
func (ts *RuntimeTransientString) PrependChar(ch rune) *RuntimeTransientString {
	ts.b.PrependChar(ch)
	return ts
}
func (ts *RuntimeTransientString) PrependString(s string) *RuntimeTransientString {
	ts.b.PrependString(s)
	return ts
}
