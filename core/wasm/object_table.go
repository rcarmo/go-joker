package wasm

import (
	"context"
	"math"
	"sync"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type ObjectTable struct {
	objects []coretypes.Object
	nilObj  coretypes.Object
	mu      sync.Mutex
}

func NewObjectTable(nilObj coretypes.Object) *ObjectTable {
	return &ObjectTable{nilObj: nilObj}
}

func (t *ObjectTable) Store(obj coretypes.Object) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	idx := len(t.objects)
	t.objects = append(t.objects, obj)
	return uint64(idx) | (1 << 62)
}

func (t *ObjectTable) Load(handle uint64) coretypes.Object {
	idx := int(handle &^ (1 << 62))
	if idx >= 0 && idx < len(t.objects) {
		return t.objects[idx]
	}
	return t.nilObj
}

func IsHandle(v uint64) bool { return v&(1<<62) != 0 }

func RawInt(v uint64) (int, bool) {
	i := int64(v)
	if i < int64(coretypes.MinInt) || i > int64(coretypes.MaxInt) {
		return 0, false
	}
	return int(i), true
}

func RawIntObject(v uint64) coretypes.Object {
	if i, ok := RawInt(v); ok {
		return coretypes.Int{I: i}
	}
	return coretypes.MakeBigInt(coretypes.MakeMathBigIntFromInt64(int64(v)))
}

func ObjToWasm(t *ObjectTable, obj coretypes.Object) uint64 {
	switch v := obj.(type) {
	case coretypes.Int:
		return uint64(v.I)
	case coretypes.Double:
		return math.Float64bits(v.D) | (1 << 63)
	default:
		return t.Store(obj)
	}
}

func WasmToObj(t *ObjectTable, v uint64) coretypes.Object {
	if IsHandle(v) {
		return t.Load(v)
	}
	if v&(1<<63) != 0 {
		return coretypes.Double{D: math.Float64frombits(v &^ (1 << 63))}
	}
	return RawIntObject(v)
}

type ctxKey struct{}

func WithObjectTable(ctx context.Context, t *ObjectTable) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

func GetObjectTable(ctx context.Context) *ObjectTable {
	if t, ok := ctx.Value(ctxKey{}).(*ObjectTable); ok {
		return t
	}
	return nil
}
