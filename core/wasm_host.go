package core

// wasm_host.go — Host function imports for WASM modules.
//
// Provides Joker collection operations (get, assoc, nth, conj, first, count)
// as imported host functions that WASM-compiled loops can call.
//
// Objects are passed as opaque handles (uint64 indices into a per-execution
// object table). Numeric values (Int, Double) are passed directly as i64/f64.
//
// The object table is thread-local to each wasmExec call, stored in a
// context value so host functions can access it.

import (
	"context"
	"math"
	"sync"

	"github.com/rcarmo/go-joker/core/internal/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// objectTable holds Joker Objects referenced by WASM code via handles.
type objectTable struct {
	objects []Object
	mu      sync.Mutex
}

// store adds an object and returns its handle.
func (t *objectTable) store(obj Object) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	idx := len(t.objects)
	t.objects = append(t.objects, obj)
	// Handles use high bit 1 to distinguish from plain i64 values
	return uint64(idx) | (1 << 62)
}

// load retrieves an object by handle.
func (t *objectTable) load(handle uint64) Object {
	idx := int(handle &^ (1 << 62))
	if idx >= 0 && idx < len(t.objects) {
		return t.objects[idx]
	}
	return NIL
}

// isHandle checks if a uint64 value is an object handle (vs plain i64).
func isHandle(v uint64) bool {
	return v&(1<<62) != 0
}

// contextKey for passing the object table through wazero context.
type ctxKey struct{}

func withObjectTable(ctx context.Context, t *objectTable) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

func getObjectTable(ctx context.Context) *objectTable {
	if t, ok := ctx.Value(ctxKey{}).(*objectTable); ok {
		return t
	}
	return nil
}

// wasmHostModuleName is the import module name for Joker host functions.
const wasmHostModuleName = wasm.HostModuleName

var wasmHostRegistered sync.Once

// registerWasmHost registers the "joker" host module with collection operations.
func registerWasmHost(rt wazero.Runtime) {
	wasmHostRegistered.Do(func() {
		ctx := context.Background()
		builder := rt.NewHostModuleBuilder(wasmHostModuleName)

		// joker.get(coll_handle, key_i64) -> result_i64_or_handle
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64, key uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return 0
				}
				coll := t.load(collHandle)
				var keyObj Object
				if isHandle(key) {
					keyObj = t.load(key)
				} else {
					keyObj = Int{I: int(int64(key))}
				}
				if g, ok := coll.(Gettable); ok {
					ok, v := g.Get(keyObj)
					if ok {
						return objToWasm(t, v)
					}
				}
				return 0 // NIL
			}).Export("get")

		// joker.get3(coll_handle, key_i64, default_i64) -> result
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64, key uint64, def uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return def
				}
				coll := t.load(collHandle)
				var keyObj Object
				if isHandle(key) {
					keyObj = t.load(key)
				} else {
					keyObj = Int{I: int(int64(key))}
				}
				if g, ok := coll.(Gettable); ok {
					ok, v := g.Get(keyObj)
					if ok {
						return objToWasm(t, v)
					}
				}
				return def
			}).Export("get3")

		// joker.assoc(coll_handle, key_i64, val_i64) -> new_coll_handle
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64, key uint64, val uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return collHandle
				}
				coll := t.load(collHandle)
				keyObj := wasmToObj(t, key)
				valObj := wasmToObj(t, val)
				if a, ok := coll.(Associative); ok {
					result := a.Assoc(keyObj, valObj)
					return t.store(result)
				}
				return collHandle
			}).Export("assoc")

		// joker.nth(coll_handle, idx_i64) -> result
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64, idx uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return 0
				}
				coll := t.load(collHandle)
				i := int(int64(idx))
				switch c := coll.(type) {
				case *ArrayVector:
					if i >= 0 && i < len(c.arr) {
						return objToWasm(t, c.arr[i])
					}
				case Indexed:
					return objToWasm(t, c.Nth(i))
				}
				return 0
			}).Export("nth")

		// joker.conj(coll_handle, val_i64) -> new_coll_handle
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64, val uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return collHandle
				}
				coll := t.load(collHandle)
				valObj := wasmToObj(t, val)
				if c, ok := coll.(Conjable); ok {
					return t.store(c.Conj(valObj))
				}
				return collHandle
			}).Export("conj")

		// joker.first(coll_handle) -> result
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return 0
				}
				coll := t.load(collHandle)
				switch v := coll.(type) {
				case *ArrayVector:
					if len(v.arr) > 0 {
						return objToWasm(t, v.arr[0])
					}
				case Seqable:
					s := v.Seq()
					if !s.IsEmpty() {
						return objToWasm(t, s.First())
					}
				}
				return 0
			}).Export("first")

		// joker.count(coll_handle) -> i64
		builder.NewFunctionBuilder().
			WithFunc(func(ctx context.Context, collHandle uint64) uint64 {
				t := getObjectTable(ctx)
				if t == nil {
					return 0
				}
				coll := t.load(collHandle)
				switch v := coll.(type) {
				case Counted:
					return uint64(v.Count())
				}
				return 0
			}).Export("count")

		builder.Instantiate(ctx)
	})
}

// objToWasm converts a Joker Object to a WASM uint64 (handle or direct value).
func objToWasm(t *objectTable, obj Object) uint64 {
	switch v := obj.(type) {
	case Int:
		return uint64(v.I)
	case Double:
		return math.Float64bits(v.D) | (1 << 63) // tag bit for float
	default:
		return t.store(obj)
	}
}

// wasmToObj converts a WASM uint64 back to a Joker Object.
func wasmToObj(t *objectTable, v uint64) Object {
	if isHandle(v) {
		return t.load(v)
	}
	if v&(1<<63) != 0 {
		// Float tagged value
		return Double{D: math.Float64frombits(v &^ (1 << 63))}
	}
	return Int{I: int(int64(v))}
}

// Ensure api import is used
var _ api.Module
