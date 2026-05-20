package wasm

import coretypes "github.com/rcarmo/go-joker/core/types"

const HostModuleName = "joker"

type HostImport struct {
	Name      string
	NumParams int // all current host imports use i64 params and i64 result
}

var StandardHostImports = []HostImport{
	{Name: "get", NumParams: 2},   // (coll, key) -> result
	{Name: "get3", NumParams: 3},  // (coll, key, default) -> result
	{Name: "assoc", NumParams: 3}, // (coll, key, val) -> new_coll
	{Name: "nth", NumParams: 2},   // (coll, idx) -> result
	{Name: "conj", NumParams: 2},  // (coll, val) -> new_coll
	{Name: "count", NumParams: 1}, // (coll) -> i64
	{Name: "first", NumParams: 1}, // (coll) -> result
}

func HostImportNames(imports []HostImport) []string {
	names := make([]string, len(imports))
	for i, imp := range imports {
		names[i] = imp.Name
	}
	return names
}

func HostGet(t *ObjectTable, collHandle uint64, key uint64, nilValue uint64) uint64 {
	if t == nil {
		return nilValue
	}
	coll := t.Load(collHandle)
	var keyObj coretypes.Object
	if IsHandle(key) {
		keyObj = t.Load(key)
	} else {
		keyObj = RawIntObject(key)
	}
	if g, ok := coll.(coretypes.Gettable); ok {
		ok, v := g.Get(keyObj)
		if ok {
			return ObjToWasm(t, v)
		}
	}
	return nilValue
}

func HostAssoc(t *ObjectTable, collHandle, key, val uint64) uint64 {
	if t == nil {
		return collHandle
	}
	coll := t.Load(collHandle)
	if a, ok := coll.(coretypes.Associative); ok {
		result := a.Assoc(WasmToObj(t, key), WasmToObj(t, val))
		return t.Store(result)
	}
	return collHandle
}

func HostNth(t *ObjectTable, collHandle, idx uint64, nilValue uint64, fastNth func(coretypes.Object, int) (coretypes.Object, bool)) uint64 {
	if t == nil {
		return nilValue
	}
	i, ok := RawInt(idx)
	if !ok {
		return nilValue
	}
	coll := t.Load(collHandle)
	if fastNth != nil {
		if v, ok := fastNth(coll, i); ok {
			return ObjToWasm(t, v)
		}
	}
	if c, ok := coll.(coretypes.Indexed); ok {
		return ObjToWasm(t, c.Nth(i))
	}
	return nilValue
}

func HostConj(t *ObjectTable, collHandle, val uint64) uint64 {
	if t == nil {
		return collHandle
	}
	coll := t.Load(collHandle)
	if c, ok := coll.(coretypes.Conjable); ok {
		return t.Store(c.Conj(WasmToObj(t, val)))
	}
	return collHandle
}

func HostFirst(t *ObjectTable, collHandle uint64, nilValue uint64, fastFirst func(coretypes.Object) (coretypes.Object, bool)) uint64 {
	if t == nil {
		return nilValue
	}
	coll := t.Load(collHandle)
	if fastFirst != nil {
		if v, ok := fastFirst(coll); ok {
			return ObjToWasm(t, v)
		}
	}
	if s, ok := coll.(coretypes.Seqable); ok {
		seq := s.Seq()
		if !seq.IsEmpty() {
			return ObjToWasm(t, seq.First())
		}
	}
	return nilValue
}

func HostCount(t *ObjectTable, collHandle uint64) uint64 {
	if t == nil {
		return 0
	}
	if c, ok := t.Load(collHandle).(coretypes.Counted); ok {
		return uint64(c.Count())
	}
	return 0
}
