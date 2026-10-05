//go:build joker_ffi && (linux || darwin || windows) && (amd64 || arm64)

// Package ffi enables explicitly typed C ABI calls for trusted Joker scripts.
package ffi

import (
	"fmt"
	"math/big"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
	core "github.com/rcarmo/go-joker/v42/core"
	corert "github.com/rcarmo/go-joker/v42/core/runtime"
	types "github.com/rcarmo/go-joker/v42/core/types"
)

type object struct{ types.InfoHolder }

func (o *object) identity() *object { return o }
func (o *object) Equals(other interface{}) bool {
	x, ok := other.(interface{ identity() *object })
	return ok && x.identity() == o
}
func (o *object) Hash() uint32         { p := uintptr(unsafe.Pointer(o)); return uint32(p ^ (p >> 32)) }
func (o *object) GetType() *types.Type { return foreignType }
func (o *object) ToString(bool) string { return "#object[FFI]" }

var foreignType = types.NewRefType("Foreign", &object{}, nil)
var libraryType = types.NewRefType("ForeignLibrary", &Library{}, nil)
var pointerType = types.NewRefType("ForeignPointer", &Pointer{}, nil)
var bufferType = types.NewRefType("ForeignBuffer", &Buffer{}, nil)
var functionType = types.NewRefType("ForeignFunction", &Function{}, nil)

func (*Library) GetType() *types.Type  { return libraryType }
func (*Pointer) GetType() *types.Type  { return pointerType }
func (*Buffer) GetType() *types.Type   { return bufferType }
func (*Function) GetType() *types.Type { return functionType }

type Library struct {
	object
	handle uintptr
	path   string
}
type Pointer struct {
	object
	address unsafe.Pointer
	library *Library
	// A result may alias an input buffer or a pointer into its memory. Retain
	// all pointer/buffer arguments conservatively, including interior aliases.
	owners []types.Object
}

func (p *Pointer) Equals(other interface{}) bool {
	x, ok := other.(*Pointer)
	return ok && p.address == x.address
}
func (p *Pointer) Hash() uint32 { x := uintptr(p.address); return uint32(x ^ (x >> 32)) }

type Buffer struct {
	object
	bytes []byte
}

func NewBuffer(n int) *Buffer {
	if n < 0 || n > 64<<20 {
		fail("buffer size must be 0..64 MiB")
	}
	return &Buffer{bytes: make([]byte, n)}
}

// Bytes is for trusted host integrations. Scripts access buffers through checked functions.
func (b *Buffer) Bytes() []byte    { return b.bytes }
func fail(format string, a ...any) { panic(core.RT.NewError("ffi: " + fmt.Sprintf(format, a...))) }
func text(o types.Object) string {
	v, ok := o.(types.String)
	if !ok {
		fail("expected string")
	}
	return v.S
}
func integer(o types.Object) int {
	v, ok := o.(types.Int)
	if !ok {
		fail("expected Int")
	}
	return v.I
}

var signatureTypes = map[string]reflect.Type{
	"i8": reflect.TypeOf(int8(0)), "u8": reflect.TypeOf(uint8(0)), "i16": reflect.TypeOf(int16(0)), "u16": reflect.TypeOf(uint16(0)),
	"i32": reflect.TypeOf(int32(0)), "u32": reflect.TypeOf(uint32(0)), "i64": reflect.TypeOf(int64(0)), "u64": reflect.TypeOf(uint64(0)),
	"size-t": reflect.TypeOf(uintptr(0)), "intptr": reflect.TypeOf(int(0)), "uintptr": reflect.TypeOf(uintptr(0)),
	"f32": reflect.TypeOf(float32(0)), "f64": reflect.TypeOf(float64(0)), "cstring": reflect.TypeOf(""), "pointer": reflect.TypeOf(unsafe.Pointer(nil)),
}

type Function struct {
	object
	library *Library
	name    string
	fn      reflect.Value
	inputs  []string
	result  string
}

func (f *Function) Call(args []types.Object) types.Object {
	core.CheckArity(args, len(f.inputs), len(f.inputs))
	vs := make([]reflect.Value, len(args))
	var keep []types.Object
	for i, a := range args {
		v := reflect.New(f.fn.Type().In(i)).Elem()
		switch v.Kind() {
		case reflect.String:
			s := text(a)
			if strings.ContainsRune(s, 0) {
				fail("embedded NUL")
			}
			v.SetString(s)
		case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
			n := int64(integer(a))
			if v.OverflowInt(n) {
				fail("integer overflow")
			}
			v.SetInt(n)
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			var n uint64
			switch x := a.(type) {
			case types.Int:
				if x.I < 0 {
					fail("unsigned overflow")
				}
				n = uint64(x.I)
			case *types.BigInt:
				if !x.B.IsUint64() {
					fail("unsigned overflow")
				}
				n = x.B.Uint64()
			default:
				fail("expected integer")
			}
			if v.OverflowUint(n) {
				fail("unsigned overflow")
			}
			v.SetUint(n)
		case reflect.Float32, reflect.Float64:
			n, ok := a.(types.Double)
			if !ok {
				fail("expected Double")
			}
			if v.OverflowFloat(n.D) {
				fail("float overflow")
			}
			v.SetFloat(n.D)
		case reflect.UnsafePointer:
			var p unsafe.Pointer
			switch x := a.(type) {
			case *Pointer:
				p = x.address
			case *Buffer:
				if len(x.bytes) > 0 {
					p = unsafe.Pointer(&x.bytes[0])
				}
			default:
				if !corert.IsNil(a) {
					fail("expected pointer, buffer or nil")
				}
			}
			v.SetPointer(p)
			keep = append(keep, a)
		}
		vs[i] = v
	}
	out := f.fn.Call(vs)
	runtime.KeepAlive(keep)
	runtime.KeepAlive(args)
	if len(out) == 0 {
		return core.NIL
	}
	v := out[0]
	switch v.Kind() {
	case reflect.String:
		return types.MakeString(v.String())
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		return types.MakeInt(int(v.Int()))
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := v.Uint()
		if u > uint64(^uint(0)>>1) {
			return types.MakeBigInt(new(big.Int).SetUint64(u))
		}
		return types.MakeInt(int(u))
	case reflect.Float32, reflect.Float64:
		return types.MakeDouble(v.Float())
	case reflect.UnsafePointer:
		if v.IsNil() {
			return core.NIL
		}
		return &Pointer{address: v.UnsafePointer(), library: f.library, owners: append([]types.Object(nil), keep...)}
	}
	fail("unsupported return")
	return nil
}
func bind(lib *Library, name string, args []string, ret string) *Function {
	if name == "" || strings.ContainsRune(name, 0) {
		fail("symbol must be nonempty and contain no NUL")
	}
	address, err := symbol(lib.handle, name)
	if err != nil {
		fail("symbol %s: %v", name, err)
	}
	in := make([]reflect.Type, len(args))
	for i, t := range args {
		x, ok := signatureTypes[t]
		if !ok {
			fail("unsupported argument type %s", t)
		}
		in[i] = x
	}
	out := []reflect.Type{}
	if ret != "void" {
		x, ok := signatureTypes[ret]
		if !ok {
			fail("unsupported return type %s", ret)
		}
		out = append(out, x)
	}
	value := reflect.New(reflect.FuncOf(in, out, false))
	func() {
		defer func() {
			if p := recover(); p != nil {
				fail("invalid signature for %s: %v", name, p)
			}
		}()
		purego.RegisterFunc(value.Interface(), address)
	}()
	return &Function{library: lib, name: name, fn: value.Elem(), inputs: args, result: ret}
}

func init() { initialize = initNamespace }
func initNamespace() {
	add := func(name, doc string, fn core.ProcFn) {
		namespace.InternVar(name, core.Proc{Fn: fn, Name: name, Package: "std/ffi"}, core.MakeMeta(nil, doc, "42.12.0"))
	}
	add("open", "Load an absolute library path. Trusted native execution; library stays loaded for process lifetime.", func(a []types.Object) types.Object {
		core.CheckArity(a, 1, 1)
		path := text(a[0])
		if strings.ContainsRune(path, 0) {
			fail("library path contains NUL")
		}
		if !filepath.IsAbs(path) {
			fail("library path must be absolute")
		}
		h, err := openLibrary(path)
		if err != nil {
			fail("open: %v", err)
		}
		return &Library{handle: h, path: path}
	})
	add("bind", "Bind a fixed C signature: (bind library symbol argument-type-vector return-type). No variadics/callbacks/structs.", func(a []types.Object) types.Object {
		core.CheckArity(a, 4, 4)
		lib, ok := a[0].(*Library)
		if !ok {
			fail("expected library")
		}
		vec, ok := a[2].(types.Vec)
		if !ok {
			fail("expected argument vector")
		}
		in := make([]string, vec.Count())
		for i := range in {
			in[i] = strings.TrimPrefix(vec.Nth(i).ToString(false), ":")
		}
		ret := strings.TrimPrefix(a[3].ToString(false), ":")
		return bind(lib, text(a[1]), in, ret)
	})
	add("buffer", "Allocate a reusable zeroed native-call byte buffer, at most 64 MiB. C must not retain its address.", func(a []types.Object) types.Object { core.CheckArity(a, 1, 1); return NewBuffer(integer(a[0])) })
	add("u32", "Read a little-endian uint32 at a checked byte offset in a buffer.", func(a []types.Object) types.Object {
		core.CheckArity(a, 2, 2)
		b, ok := a[0].(*Buffer)
		if !ok {
			fail("expected buffer")
		}
		i := integer(a[1])
		if i < 0 || i > len(b.bytes)-4 {
			fail("buffer bounds")
		}
		return types.MakeInt(int(uint32(b.bytes[i]) | uint32(b.bytes[i+1])<<8 | uint32(b.bytes[i+2])<<16 | uint32(b.bytes[i+3])<<24))
	})
}
