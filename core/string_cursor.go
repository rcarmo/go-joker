package core

import (
	"unicode/utf8"

	corestr "github.com/rcarmo/go-joker/core/string"
)

// StringCursor is an efficient iterator over a string's runes.
// It maintains byte offset and rune index for O(1) advance/char access.
// Implements Object interface for use in Joker code.
type StringCursor struct {
	InfoHolder
	s         string // underlying string
	byteOff   int    // current byte offset
	runeIndex int    // current rune index (for user-facing position)
	runeCount int    // total rune count (cached)
	ascii     bool   // true if string is pure ASCII (O(1) indexing)
}

func NewStringCursor(s string) *StringCursor {
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
	return &StringCursor{
		s:         s,
		byteOff:   0,
		runeIndex: 0,
		runeCount: runeCount,
		ascii:     ascii,
	}
}

func (c *StringCursor) Done() bool {
	return c.byteOff >= len(c.s)
}

func (c *StringCursor) Char() rune {
	if c.byteOff >= len(c.s) {
		return -1
	}
	if c.ascii {
		return rune(c.s[c.byteOff])
	}
	r, _ := utf8.DecodeRuneInString(c.s[c.byteOff:])
	return r
}

func (c *StringCursor) Next() *StringCursor {
	if c.byteOff >= len(c.s) {
		return c
	}
	next := &StringCursor{
		s:         c.s,
		byteOff:   c.byteOff,
		runeIndex: c.runeIndex + 1,
		runeCount: c.runeCount,
		ascii:     c.ascii,
	}
	if c.ascii {
		next.byteOff++
	} else {
		_, size := utf8.DecodeRuneInString(c.s[c.byteOff:])
		next.byteOff += size
	}
	return next
}

// Index returns the current rune index.
func (c *StringCursor) Index() int {
	return c.runeIndex
}

// --- Object interface ---

func (c *StringCursor) ToString(escape bool) string {
	return "#<StringCursor>"
}

func (c *StringCursor) Equals(other interface{}) bool {
	if o, ok := other.(*StringCursor); ok {
		return c.s == o.s && c.byteOff == o.byteOff
	}
	return false
}

func (c *StringCursor) GetInfo() *ObjectInfo {
	return nil
}

func (c *StringCursor) Hash() uint32 {
	return uint32(c.byteOff) ^ corestr.Hash(c.s)
}

func (c *StringCursor) WithInfo(info *ObjectInfo) Object {
	return c
}

func (c *StringCursor) GetType() *Type {
	return typeStringCursor
}

var typeStringCursor = &Type{
	name: "StringCursor",
}
