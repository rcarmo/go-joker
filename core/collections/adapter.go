package collections

type ConstructionAdapter[Object any, Seq any, List any, Vector any, ArrayVector any, ArrayMap any, HashMap any, MapSet any] struct {
	EmptyList        func() List
	ListFrom         func(...Object) List
	EmptyVector      func() Vector
	VectorFrom       func(...Object) Vector
	VectorFromSeq    func(Seq) Vector
	EmptyArrayVector func() ArrayVector
	ArrayVectorFrom  func(...Object) ArrayVector
	EmptyArrayMap    func() ArrayMap
	HashMapFrom      func(...Object) HashMap
	EmptySet         func() MapSet
	SetFromSeq       func(Seq) MapSet
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewEmptyList() List {
	return a.EmptyList()
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewListFrom(objs ...Object) List {
	return a.ListFrom(objs...)
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewEmptyVector() Vector {
	return a.EmptyVector()
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewVectorFrom(objs ...Object) Vector {
	return a.VectorFrom(objs...)
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewVectorFromSeq(seq Seq) Vector {
	return a.VectorFromSeq(seq)
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewEmptyArrayVector() ArrayVector {
	return a.EmptyArrayVector()
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewArrayVectorFrom(objs ...Object) ArrayVector {
	return a.ArrayVectorFrom(objs...)
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewEmptyArrayMap() ArrayMap {
	return a.EmptyArrayMap()
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewHashMapFrom(keyvals ...Object) HashMap {
	return a.HashMapFrom(keyvals...)
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewEmptySet() MapSet {
	return a.EmptySet()
}

func (a ConstructionAdapter[Object, Seq, List, Vector, ArrayVector, ArrayMap, HashMap, MapSet]) NewSetFromSeq(seq Seq) MapSet {
	return a.SetFromSeq(seq)
}
