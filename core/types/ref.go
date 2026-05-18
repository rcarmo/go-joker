package types

type Ref interface {
	AlterMeta(fn Callable, args []Object) Map
	ResetMeta(m Map) Map
}
