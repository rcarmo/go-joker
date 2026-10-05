//go:build (linux || darwin || windows) && (amd64 || arm64)

package ffi

import (
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	core "github.com/rcarmo/go-joker/v42/core"
	corert "github.com/rcarmo/go-joker/v42/core/runtime"
	types "github.com/rcarmo/go-joker/v42/core/types"
	collections "github.com/rcarmo/go-joker/v42/core/types/collections"
)

func fixture(t *testing.T) *Library {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("native DLL fixture requires Windows compiler qualification")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C fixture compiler unavailable (Go caller is no-cgo)")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "fixture.c")
	lib := filepath.Join(dir, "fixture.so")
	c := `#include <stdint.h>
#include <stddef.h>
#include <string.h>
int32_t add(int32_t a,int32_t b){return a+b;}
uint64_t wide(uint64_t x){return x;}
double fp(double x){return x*2;}
void fill(void *p, uint32_t n){if(n)memset(p,42,n);}
static int32_t count=0;
int32_t tick(void){return ++count;}
int32_t ticks(void){return count;}
void* nilptr(void){return 0;}
void* alias(void* p){return p;}
`
	if err := os.WriteFile(source, []byte(c), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-shared", "-fPIC", source, "-o", lib}
	if runtime.GOOS == "darwin" {
		args = []string{"-dynamiclib", source, "-o", lib}
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	h, err := openLibrary(lib)
	if err != nil {
		t.Fatal(err)
	}
	return &Library{handle: h, path: lib}
}
func TestFFIFixedABI(t *testing.T) {
	lib := fixture(t)
	add := bind(lib, "add", []string{"i32", "i32"}, "i32")
	if got := add.Call([]types.Object{types.MakeInt(20), types.MakeInt(22)}).(types.Int).I; got != 42 {
		t.Fatal(got)
	}
	other := bind(lib, "fp", []string{"f64"}, "f64")
	if add.Equals(other) || !add.Equals(add) {
		t.Fatal("callable identity")
	}
	if got := other.Call([]types.Object{types.MakeDouble(21)}).(types.Double).D; got != 42 {
		t.Fatal(got)
	}
	u := new(big.Int).SetUint64(math.MaxUint64)
	wide := bind(lib, "wide", []string{"u64"}, "u64")
	if got := wide.Call([]types.Object{types.MakeBigInt(u)}).(*types.BigInt); got.B.Cmp(u) != 0 {
		t.Fatal(got)
	}
	buffer := NewBuffer(64)
	fill := bind(lib, "fill", []string{"pointer", "u32"}, "void")
	fill.Call([]types.Object{buffer, types.MakeInt(64)})
	for _, b := range buffer.bytes {
		if b != 42 {
			t.Fatal("buffer fill")
		}
	}
	if !corert.IsNil(bind(lib, "nilptr", nil, "pointer").Call(nil)) {
		t.Fatal("NULL must return nil")
	}
	alias := bind(lib, "alias", []string{"pointer"}, "pointer")
	a := alias.Call([]types.Object{buffer}).(*Pointer)
	if len(a.owners) != 1 || a.owners[0] != buffer {
		t.Fatal("aliased buffer owner lost")
	}
	b := alias.Call([]types.Object{a}).(*Pointer)
	if !a.Equals(b) || a.Hash() != b.Hash() {
		t.Fatal("native pointer equality")
	}
	assertPanics(t, func() { add.Call([]types.Object{types.MakeInt(1 << 40), types.MakeInt(0)}) })
	assertPanics(t, func() { add.Call(nil) })
	assertPanics(t, func() { bind(lib, "missing", nil, "void") })
	assertPanics(t, func() { bind(lib, "add", []string{"struct"}, "i32") })
	// Object identity/hash must support use as map keys; distinct foreign callables.
	m := collections.EmptyArrayMap().Assoc(add, types.MakeInt(1)).Assoc(other, types.MakeInt(2))
	if m.(types.Counted).Count() != 2 {
		t.Fatal("foreign map keys collapsed")
	}
}
func TestFFIPointerNilAndBounds(t *testing.T) {
	initNamespace()
	core.GLOBAL_ENV.InitEnv(os.Stdin, os.Stdout, os.Stderr, nil)
	lib := fixture(t)
	fn := bind(lib, "fill", []string{"pointer", "u32"}, "void")
	fn.Call([]types.Object{corert.Nil{}, types.MakeInt(0)})
	assertPanics(t, func() { fn.Call([]types.Object{types.MakeInt(123), types.MakeInt(0)}) })
	assertPanics(t, func() { NewBuffer(-1) })
	// Byte readers reject both negative and end-overrun offsets.
	ns := core.GLOBAL_ENV.EnsureSymbolIsLib(types.MakeSymbol(core.STRINGS.Intern, "joker.ffi"))
	v := ns.Resolve("u32")
	if v == nil {
		t.Fatal("u32 var unavailable")
	}
	assertPanics(t, func() { v.Value.(core.Proc).Call([]types.Object{NewBuffer(4), types.MakeInt(1)}) })
	assertPanics(t, func() { v.Value.(core.Proc).Call([]types.Object{NewBuffer(4), types.MakeInt(-1)}) })
	assertPanics(t, func() { bind(lib, "add\x00bad", []string{"i32", "i32"}, "i32") })
}
func TestFFINoReplayAfterNativeEffect(t *testing.T) {
	lib := fixture(t)
	tick := bind(lib, "tick", nil, "i32")
	ticks := bind(lib, "ticks", nil, "i32")
	ns := core.GLOBAL_ENV.EnsureSymbolIsLib(types.MakeSymbol(core.STRINGS.Intern, "ffi.test"))
	ns.InternVar("tick", tick, core.MakeMeta(nil, "native counter", ""))
	// Mixed call and overflowing multiplication: native effect must occur once.
	obj, err := core.TryRead(core.NewReader(strings.NewReader("(do (ffi.test/tick) (* 9223372036854775807 2))"), "<ffi-test>"))
	if err != nil {
		t.Fatal(err)
	}
	expr, err := core.TryParse(obj, &core.ParseContext{GlobalEnv: core.GLOBAL_ENV})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = core.TryEval(expr); err != nil {
		t.Fatal(err)
	}
	if got := ticks.Call(nil).(types.Int).I; got != 1 {
		t.Fatalf("foreign side effect replayed: %d", got)
	}
}
func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected validation panic")
		}
	}()
	fn()
}
