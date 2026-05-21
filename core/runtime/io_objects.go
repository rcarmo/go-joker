package runtime

import (
	"bytes"
	"errors"
	"io"
	"os"

	corereader "github.com/rcarmo/go-joker/core/reader"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

type File struct{ *corereader.File }

func MakeFile(f *os.File) *File { return &File{File: corereader.NewFile(f)} }

func (f *File) ToString(escape bool) string                          { return "#object[File]" }
func (f *File) Equals(other interface{}) bool                        { return f == other }
func (f *File) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (f *File) GetType() *coretypes.Type                             { return coretypes.RuntimeTypes.File }
func (f *File) Hash() uint32                                         { return f.File.Hash() }
func (f *File) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return f }
func (f *File) Namespace() string                                    { return "" }

type Buffer struct{ *corereader.Buffer }

func MakeBuffer(b *bytes.Buffer) *Buffer { return &Buffer{Buffer: corereader.NewBuffer(b)} }

func (b *Buffer) ToString(escape bool) string                          { return b.String() }
func (b *Buffer) Equals(other interface{}) bool                        { return b == other }
func (b *Buffer) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (b *Buffer) GetType() *coretypes.Type                             { return coretypes.RuntimeTypes.Buffer }
func (b *Buffer) Hash() uint32                                         { return b.Buffer.Hash() }
func (b *Buffer) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return b }

type BufferedReader struct{ *corereader.Buffered }

func MakeBufferedReader(rd io.Reader) *BufferedReader {
	return &BufferedReader{Buffered: corereader.NewBuffered(rd)}
}

func (br *BufferedReader) ToString(escape bool) string                          { return "#object[BufferedReader]" }
func (br *BufferedReader) Equals(other interface{}) bool                        { return br == other }
func (br *BufferedReader) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (br *BufferedReader) GetType() *coretypes.Type                             { return coretypes.RuntimeTypes.BufferedReader }
func (br *BufferedReader) Hash() uint32                                         { return br.Buffered.Hash() }
func (br *BufferedReader) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return br }

type IOReader struct{ *corereader.IOReader }

func MakeIOReader(r io.Reader) *IOReader { return &IOReader{IOReader: corereader.NewIOReader(r)} }

func (ior *IOReader) ToString(escape bool) string                          { return "#object[IOReader]" }
func (ior *IOReader) Equals(other interface{}) bool                        { return ior == other }
func (ior *IOReader) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (ior *IOReader) GetType() *coretypes.Type                             { return coretypes.RuntimeTypes.IOReader }
func (ior *IOReader) Hash() uint32                                         { return ior.IOReader.Hash() }
func (ior *IOReader) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return ior }
func (ior *IOReader) Close() error {
	if err := ior.IOReader.Close(); err != nil {
		if errors.Is(err, corereader.ErrNotClosable) {
			return coretypes.RuntimeError("coretypes.Object is not closable: " + ior.ToString(false)).(error)
		}
		return err
	}
	return nil
}

type IOWriter struct{ *corereader.IOWriter }

func MakeIOWriter(w io.Writer) *IOWriter { return &IOWriter{IOWriter: corereader.NewIOWriter(w)} }

func (iow *IOWriter) ToString(escape bool) string                          { return "#object[IOWriter]" }
func (iow *IOWriter) Equals(other interface{}) bool                        { return iow == other }
func (iow *IOWriter) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (iow *IOWriter) GetType() *coretypes.Type                             { return coretypes.RuntimeTypes.IOWriter }
func (iow *IOWriter) Hash() uint32                                         { return iow.IOWriter.Hash() }
func (iow *IOWriter) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return iow }
func (iow *IOWriter) Close() error {
	if err := iow.IOWriter.Close(); err != nil {
		if errors.Is(err, corereader.ErrNotClosable) {
			return coretypes.RuntimeError("coretypes.Object is not closable: " + iow.ToString(false)).(error)
		}
		return err
	}
	return nil
}
