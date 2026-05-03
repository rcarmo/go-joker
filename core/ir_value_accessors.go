package core

import (
	"unsafe"
)

// ir_value_accessors.go — typed accessors for irValue's unsafe.Pointer field.
//
// irValue stores extended data (strings, collections, objects) behind an
// unsafe.Pointer to keep the struct at 32 bytes for the numeric hot path.
// These accessors provide type-safe reads/writes.

// --- String ---

func irMakeString(s string, runeCount int, ascii bool) irValue {
	v := irValue{tag: irValString, i: runeCount, p: unsafe.Pointer(&s)}
	if ascii {
		v.f = 1
	}
	return v
}

func (v irValue) str() string {
	if v.p == nil {
		return ""
	}
	return *(*string)(v.p)
}

func (v irValue) isASCII() bool { return v.f != 0 }

// --- StringBuilder ([]byte) ---

func irMakeStringBuilder(buf []byte, runeCount int, ascii bool) irValue {
	v := irValue{tag: irValStringBuilder, i: runeCount, p: unsafe.Pointer(&buf)}
	if ascii {
		v.f = 1
	}
	return v
}

func (v irValue) bytes() []byte {
	if v.p == nil {
		return nil
	}
	return *(*[]byte)(v.p)
}

func (v *irValue) setBytes(buf []byte) {
	v.p = unsafe.Pointer(&buf)
}

func (v *irValue) setASCII(ascii bool) {
	if ascii {
		v.f = 1
	} else {
		v.f = 0
	}
}

// --- Bool ---

func irMakeBool(b bool) irValue {
	v := irValue{tag: irValBool}
	if b {
		v.i = 1
	}
	return v
}

func (v irValue) boolean() bool { return v.i != 0 }

// --- Char ---

func irMakeChar(r rune) irValue {
	return irValue{tag: irValChar, i: int(r)}
}

func (v irValue) char() rune { return rune(v.i) }

// --- StringIntMap ---

func irMakeStringIntMap(m map[string]int) irValue {
	return irValue{tag: irValStringIntMap, p: unsafe.Pointer(&m)}
}

func (v irValue) stringIntMap() map[string]int {
	if v.p == nil {
		return nil
	}
	return *(*map[string]int)(v.p)
}

func (v *irValue) setStringIntMap(m map[string]int) {
	v.p = unsafe.Pointer(&m)
}

// --- IntVector ---

func irMakeIntVector(iv []int) irValue {
	return irValue{tag: irValIntVector, p: unsafe.Pointer(&iv)}
}

func (v irValue) intVec() []int {
	if v.p == nil {
		return nil
	}
	return *(*[]int)(v.p)
}

func (v *irValue) setIntVec(iv []int) {
	v.p = unsafe.Pointer(&iv)
}

// --- Object ---

func irMakeObject(obj Object) irValue {
	// For common concrete pointer types, store directly to avoid
	// allocating an Object interface box. Use i field as sub-tag.
	switch v := obj.(type) {
	case *ArrayVector:
		return irValue{tag: irValObject, i: 1, p: unsafe.Pointer(v)}
	case *TransientVector:
		return irValue{tag: irValObject, i: 2, p: unsafe.Pointer(v)}
	case *Fn:
		return irValue{tag: irValObject, i: 3, p: unsafe.Pointer(v)}
	default:
		p := new(Object)
		*p = obj
		return irValue{tag: irValObject, i: 0, p: unsafe.Pointer(p)}
	}
}

func (v irValue) obj() Object {
	if v.p == nil {
		return NIL
	}
	switch v.i {
	case 1:
		return (*ArrayVector)(v.p)
	case 2:
		return (*TransientVector)(v.p)
	case 3:
		return (*Fn)(v.p)
	default:
		return *(*Object)(v.p)
	}
}
