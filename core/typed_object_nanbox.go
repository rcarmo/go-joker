package core

import coreirx "github.com/rcarmo/go-joker/core/ir"

func nbFromObject(obj Object, table *[]Object) uint64 {
	switch v := obj.(type) {
	case Int:
		return coreirx.BoxInt(v.I)
	case Double:
		return coreirx.BoxDouble(v.D)
	case Boolean:
		return coreirx.BoxBool(v.B)
	case Nil:
		return coreirx.BoxNil()
	default:
		idx := len(*table)
		*table = append(*table, obj)
		return coreirx.BoxObj(idx)
	}
}

func nbToObject(v uint64, table []Object) Object {
	if coreirx.IsDouble(v) {
		return Double{D: coreirx.ToDouble(v)}
	}
	if coreirx.IsInt(v) {
		return Int{I: coreirx.ToInt(v)}
	}
	if coreirx.IsBool(v) {
		return Boolean{B: coreirx.ToBool(v)}
	}
	if coreirx.IsNil(v) {
		return NIL
	}
	if coreirx.IsObj(v) {
		idx := coreirx.ToObjIdx(v)
		if idx < len(table) {
			return table[idx]
		}
	}
	return NIL
}
