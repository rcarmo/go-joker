package string

import (
	"hash/fnv"
	"unicode/utf8"
)

// Cursor is an efficient iterator over a string's runes.
type Cursor struct {
	s         string
	byteOff   int
	runeIndex int
	runeCount int
	ascii     bool
}

func NewCursor(s string) *Cursor {
	ascii := true
	runeCount := 0
	for i := 0; i < len(s); {
		if s[i] < 0x80 {
			i++
			runeCount++
		} else {
			ascii = false
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
			runeCount++
		}
	}
	return &Cursor{s: s, byteOff: 0, runeIndex: 0, runeCount: runeCount, ascii: ascii}
}

func (c *Cursor) Done() bool { return c.byteOff >= len(c.s) }
func (c *Cursor) Char() rune {
	if c.byteOff >= len(c.s) {
		return -1
	}
	if c.ascii {
		return rune(c.s[c.byteOff])
	}
	r, _ := utf8.DecodeRuneInString(c.s[c.byteOff:])
	return r
}
func (c *Cursor) Next() *Cursor {
	if c.byteOff >= len(c.s) {
		return c
	}
	next := &Cursor{s: c.s, byteOff: c.byteOff, runeIndex: c.runeIndex + 1, runeCount: c.runeCount, ascii: c.ascii}
	if c.ascii {
		next.byteOff++
	} else {
		_, size := utf8.DecodeRuneInString(c.s[c.byteOff:])
		next.byteOff += size
	}
	return next
}
func (c *Cursor) Index() int           { return c.runeIndex }
func (c *Cursor) String() string       { return "#<StringCursor>" }
func (c *Cursor) Equal(o *Cursor) bool { return o != nil && c.s == o.s && c.byteOff == o.byteOff }
func (c *Cursor) Hash() uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(c.s))
	return uint32(c.byteOff) ^ h.Sum32()
}

type CursorRuntime struct{ cur *Cursor }

func NewCursorRuntime(s string) *CursorRuntime { return &CursorRuntime{cur: NewCursor(s)} }
func (c *CursorRuntime) Done() bool            { return c.cur.Done() }
func (c *CursorRuntime) Char() rune            { return c.cur.Char() }
func (c *CursorRuntime) Index() int            { return c.cur.Index() }
func (c *CursorRuntime) String() string        { return c.cur.String() }
func (c *CursorRuntime) Hash() uint32          { return c.cur.Hash() }
func (c *CursorRuntime) Next() *CursorRuntime {
	next := c.cur.Next()
	if next == c.cur {
		return c
	}
	return &CursorRuntime{cur: next}
}
func (c *CursorRuntime) Equal(other *CursorRuntime) bool {
	return other != nil && c.cur.Equal(other.cur)
}
