package cursor

import (
	"unicode/utf8"

	corestr "github.com/rcarmo/go-joker/core/string"
)

// Cursor is an efficient iterator over a string's runes.
// It maintains byte offset and rune index for O(1) advance/char access.
// Implements Object interface for use in Joker code.
type Cursor struct {
	s         string // underlying string
	byteOff   int    // current byte offset
	runeIndex int    // current rune index (for user-facing position)
	runeCount int    // total rune count (cached)
	ascii     bool   // true if string is pure ASCII (O(1) indexing)
}

func New(s string) *Cursor {
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
	return &Cursor{
		s:         s,
		byteOff:   0,
		runeIndex: 0,
		runeCount: runeCount,
		ascii:     ascii,
	}
}

func (c *Cursor) Done() bool {
	return c.byteOff >= len(c.s)
}

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
	next := &Cursor{
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
func (c *Cursor) Index() int {
	return c.runeIndex
}

func (c *Cursor) String() string { return "#<StringCursor>" }

func (c *Cursor) Equal(o *Cursor) bool {
	return o != nil && c.s == o.s && c.byteOff == o.byteOff
}

func (c *Cursor) Hash() uint32 {
	return uint32(c.byteOff) ^ corestr.Hash(c.s)
}
