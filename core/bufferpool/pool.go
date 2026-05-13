package bufferpool

import (
	"bytes"
	"sync"
)

var pool = sync.Pool{
	New: func() any { return &bytes.Buffer{} },
}

func Get() *bytes.Buffer {
	b := pool.Get().(*bytes.Buffer)
	b.Reset()
	return b
}

func Put(b *bytes.Buffer) {
	b.Reset()
	pool.Put(b)
}
