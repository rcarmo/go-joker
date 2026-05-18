package types

var RuntimeNil Object
var RuntimeReduceType *Type
var RuntimeKVReduceType *Type
var SpecialSymbolLookup func(Symbol) bool

func IsInstance(t *Type, obj Object) bool {
	if RuntimeNil != nil && obj.Equals(RuntimeNil) {
		return false
	}
	if RuntimeReduceType != nil && t == RuntimeReduceType {
		_, ok := obj.(Reduce)
		return ok
	}
	if RuntimeKVReduceType != nil && t == RuntimeKVReduceType {
		_, ok := obj.(KVReduce)
		return ok
	}
	return IsEqualOrImplements(t, obj.GetType())
}

func IsSpecialSymbol(obj Object) bool {
	sym, ok := obj.(Symbol)
	if !ok || sym.NamespaceKey() != nil {
		return false
	}
	if SpecialSymbolLookup == nil {
		return false
	}
	return SpecialSymbolLookup(sym)
}
