package types

func IsSymbol(obj Object) bool {
	_, ok := obj.(Symbol)
	return ok
}

func IsKeyword(obj Object) bool {
	_, ok := obj.(Keyword)
	return ok
}

func IsVector(obj Object) bool {
	_, ok := obj.(Vec)
	return ok
}

func IsSeq(obj Object) bool {
	_, ok := obj.(Seq)
	return ok
}
