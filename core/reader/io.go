package reader

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
)

type Buffer struct {
	*bytes.Buffer
	hash uint32
}

func NewBuffer(b *bytes.Buffer) *Buffer {
	res := &Buffer{Buffer: b}
	res.hash = hashutil.Ptr(uintptr(unsafe.Pointer(res)))
	return res
}

func (b *Buffer) Hash() uint32 { return b.hash }

type File struct {
	*os.File
	hash uint32
}

func NewFile(f *os.File) *File {
	res := &File{File: f}
	res.hash = hashutil.Ptr(uintptr(unsafe.Pointer(res)))
	return res
}

func (f *File) Hash() uint32 { return f.hash }

type Buffered struct {
	*bufio.Reader
	hash uint32
}

func NewBuffered(rd io.Reader) *Buffered {
	res := &Buffered{Reader: bufio.NewReader(rd)}
	res.hash = hashutil.Ptr(uintptr(unsafe.Pointer(res)))
	return res
}

func (br *Buffered) Hash() uint32 { return br.hash }

type IOReader struct {
	io.Reader
	hash uint32
}

func NewIOReader(r io.Reader) *IOReader {
	res := &IOReader{Reader: r}
	res.hash = hashutil.Ptr(uintptr(unsafe.Pointer(res)))
	return res
}

func (ior *IOReader) Hash() uint32 { return ior.hash }

func (ior *IOReader) Close() error {
	if c, ok := ior.Reader.(io.Closer); ok {
		return c.Close()
	}
	return ErrNotClosable
}

type IOWriter struct {
	io.Writer
	hash uint32
}

func NewIOWriter(w io.Writer) *IOWriter {
	res := &IOWriter{Writer: w}
	res.hash = hashutil.Ptr(uintptr(unsafe.Pointer(res)))
	return res
}

func (iow *IOWriter) Hash() uint32 { return iow.hash }

func (iow *IOWriter) Close() error {
	if c, ok := iow.Writer.(io.Closer); ok {
		return c.Close()
	}
	return ErrNotClosable
}

var ErrNotClosable = errNotClosable{}

type errNotClosable struct{}

func (errNotClosable) Error() string { return "object is not closable" }
