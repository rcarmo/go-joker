package types

type Set interface {
	Conjable
	Gettable
	Disjoin(key Object) Set
}
