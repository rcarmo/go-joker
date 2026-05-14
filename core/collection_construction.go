package core

// CollectionConstructionAdapter is the narrow root-owned construction surface
// for collection values. Future extraction of vectors/maps/sets should route
// through this surface instead of scattering concrete constructor calls across
// evaluator code.
type CollectionConstructionAdapter struct{}

var collectionConstruction CollectionConstructionAdapter

func (CollectionConstructionAdapter) EmptyVector() *Vector {
	return EmptyVector()
}

func (CollectionConstructionAdapter) VectorFrom(objs ...Object) *Vector {
	return NewVectorFrom(objs...)
}

func (CollectionConstructionAdapter) VectorFromSeq(seq Seq) *Vector {
	return NewVectorFromSeq(seq)
}

func (CollectionConstructionAdapter) EmptyArrayVector() *ArrayVector {
	return EmptyArrayVector()
}

func (CollectionConstructionAdapter) ArrayVectorFrom(objs ...Object) *ArrayVector {
	return NewArrayVectorFrom(objs...)
}

func (CollectionConstructionAdapter) EmptyArrayMap() *ArrayMap {
	return EmptyArrayMap()
}

func (CollectionConstructionAdapter) HashMapFrom(keyvals ...Object) *HashMap {
	return NewHashMap(keyvals...)
}

func (CollectionConstructionAdapter) EmptySet() *MapSet {
	return EmptySet()
}

func (CollectionConstructionAdapter) SetFromSeq(seq Seq) *MapSet {
	return NewSetFromSeq(seq)
}
