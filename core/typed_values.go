package core

import (
	"strconv"
	"sync"
	"unicode/utf8"
	"unsafe"

	corert "github.com/rcarmo/go-joker/core/runtime"
)

// ir_typed.go — experimental typed IR executor (v2).
//
// This is the first incremental step away from the boxed []Object stack used by
// irExec. It is intentionally small and gated: primitive/string-only loops can
// be executed with tagged values, while unsupported opcodes return nil and let
// the normal IR/tree path handle them.

type irValueTag byte

const (
	irValObject irValueTag = iota
	irValInt
	irValDouble
	irValBool
	irValChar
	irValString
	irValStringBuilder
	irValStringIntMap
	irValIntVector
	irValNil
	irValKeyword
	irValCursor // StringCursor pointer in obj field
)

// irValue is the tagged value for the typed IR executor.
// Layout: 32 bytes for the compact numeric path.
// String/collection data is stored behind an unsafe.Pointer to avoid
// bloating the struct for the common numeric case.
type irValue struct {
	tag irValueTag
	i   int            // int value, bool (0/1), rune, rune count for strings
	f   float64        // double value, or ASCII flag (nonzero = ASCII) for strings
	p   unsafe.Pointer // -> string | []byte | map[string]int | []int | Object
}

func irTypedEligible(a IRAnalysis) bool {
	if a.NumOps == 0 || a.UsesTransient {
		return false
	}
	// Call-slot loops: allow if numeric-only or numeric+generic-nth
	if a.HasCallSlot {
		return !a.UsesString && !a.HasMapOps && (!a.UsesCollection || a.HasGenericNth)
	}
	// Collection programs with nth but NO assoc (read-only vector access)
	if a.UsesCollection && a.HasGenericNth && !a.HasMapOps && !a.UsesString && !a.HasAssoc {
		return true
	}
	// Collection programs with assoc: prefer boxed executor (has transient support)
	if a.UsesCollection && a.HasGenericNth && a.HasAssoc && !a.HasMapOps && !a.UsesString {
		return false
	}
	if a.UsesCollection && (a.HasMapOps || !a.HasGenericNth) {
		if corert.IRTypedMapEnabled() && a.HasMapOps && a.UsesString {
			return true
		}
		// Self-recursive tree builders/walkers (binary-trees pattern)
		if a.HasSelfCall && !a.HasMapOps && !a.UsesString {
			return true
		}
		return corert.IRTypedVecEnabled() && a.UsesCollection && !a.UsesString && !a.HasMapOps
	}
	// Accept: pure numeric loops (no strings, no collections, no call-slots)
	if !a.UsesString && !a.UsesCollection && !a.HasCallSlot {
		return true
	}
	return a.UsesString || a.SuggestedPath == "typed-ir-string-candidate" || a.SuggestedPath == "typed-ir-generic-string-nth-candidate"
}

func stringToIRValue(s string) irValue {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			return irMakeString(s, utf8.RuneCountInString(s), false)
		}
	}
	return irMakeString(s, len(s), ascii)
}

func objectToIRValue(obj Object) irValue {
	switch v := obj.(type) {
	case Int:
		return irValue{tag: irValInt, i: v.I}
	case Double:
		return irValue{tag: irValDouble, f: v.D}
	case Boolean:
		return irMakeBool(v.B)
	case Char:
		return irMakeChar(v.Ch)
	case String:
		return stringToIRValue(v.S)
	case *ArrayVector:
		if corert.IRTypedVecEnabled() {
			iv := make([]int, len(v.arr))
			for i, obj := range v.arr {
				x, ok := obj.(Int)
				if !ok {
					return irMakeObject(obj)
				}
				iv[i] = x.I
			}
			return irMakeIntVector(iv)
		}
	case *ArrayMap:
		if v.Count() == 0 {
			return irMakeStringIntMap(make(map[string]int))
		}
	case *HashMap:
		if v.Count() == 0 {
			return irMakeStringIntMap(make(map[string]int))
		}
	case Nil:
		return irValue{tag: irValNil}
	case Keyword:
		return irValue{tag: irValKeyword, p: unsafe.Pointer(v.name)}
	case *StringCursor:
		return irValue{tag: irValCursor, p: unsafe.Pointer(v)}
	default:
		return irMakeObject(obj)
	}
	return irMakeObject(obj)
}

func (v irValue) object() Object {
	switch v.tag {
	case irValInt:
		return Int{I: v.i}
	case irValDouble:
		return Double{D: v.f}
	case irValBool:
		return Boolean{B: v.boolean()}
	case irValChar:
		return Char{Ch: v.char()}
	case irValString:
		return String{S: v.str()}
	case irValStringBuilder:
		return String{S: string(v.bytes())}
	case irValStringIntMap:
		res := collectionConstruction.EmptyArrayMap()
		for k, v := range v.stringIntMap() {
			res.Add(String{S: k}, Int{I: v})
		}
		return res
	case irValIntVector:
		arr := make([]Object, len(v.intVec()))
		for i, x := range v.intVec() {
			arr[i] = Int{I: x}
		}
		return &ArrayVector{arr: arr}
	case irValNil:
		return NIL
	case irValKeyword:
		return keywordObjectFromName((*string)(v.p))
	case irValCursor:
		return (*StringCursor)(v.p)
	default:
		if v.obj() == nil {
			return NIL
		}
		return v.obj()
	}
}

func (v irValue) truthy() bool {
	switch v.tag {
	case irValBool:
		return v.boolean()
	case irValNil:
		return false
	default:
		return true
	}
}

func irValueToString(v irValue) string {
	switch v.tag {
	case irValString:
		return v.str()
	case irValStringBuilder:
		return string(v.bytes())
	case irValChar:
		return charToStringFast(v.char())
	case irValNil:
		return ""
	case irValInt:
		return strconv.Itoa(v.i)
	case irValDouble:
		return strconv.FormatFloat(v.f, 'g', -1, 64)
	case irValBool:
		if v.boolean() {
			return "true"
		}
		return "false"
	default:
		return v.object().ToString(false)
	}
}

func irValueStringKey(v irValue) (string, bool) {
	switch v.tag {
	case irValString:
		return v.str(), true
	case irValStringBuilder:
		return string(v.bytes()), true
	case irValChar:
		return charToStringFast(v.char()), true
	default:
		return "", false
	}
}

func irStringRuneCount(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return utf8.RuneCountInString(s)
		}
	}
	return len(s)
}

func irValueEq(a, b irValue) (irValue, bool) {
	if a.tag == b.tag {
		switch a.tag {
		case irValInt:
			return irMakeBool(a.i == b.i), true
		case irValDouble:
			return irMakeBool(a.f == b.f), true
		case irValBool:
			return irMakeBool(a.boolean() == b.boolean()), true
		case irValChar:
			return irMakeBool(a.char() == b.char()), true
		case irValString:
			return irMakeBool(a.str() == b.str()), true
		case irValStringBuilder:
			return irMakeBool(string(a.bytes()) == string(b.bytes())), true
		case irValNil:
			return irMakeBool(true), true
		case irValKeyword:
			// Keywords are interned — pointer equality on name
			return irMakeBool(a.p == b.p), true
		}
	}
	if a.tag == irValInt && b.tag == irValDouble {
		return irMakeBool(float64(a.i) == b.f), true
	}
	if a.tag == irValDouble && b.tag == irValInt {
		return irMakeBool(a.f == float64(b.i)), true
	}
	return irMakeBool(a.object().Equals(b.object())), true
}

// keywordObjectCache caches Keyword Objects by name pointer to avoid
// repeated heap allocation when converting irValKeyword → Object.
var keywordObjectCache sync.Map // *string → Object (Keyword)

func keywordObjectFromName(name *string) Object {
	if v, ok := keywordObjectCache.Load(name); ok {
		return v.(Object)
	}
	kw := Keyword{name: name}
	// Store as Object interface to avoid re-boxing
	var obj Object = kw
	keywordObjectCache.Store(name, obj)
	return obj
}
