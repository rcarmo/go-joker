package types

type MetaFactory func(kind Kind, name string, doc string) any

type Builder struct {
	Registry    Registry
	Intern      func(string) *string
	MetaFactory MetaFactory
}

func (b Builder) RegisterReference(name string, inst any, doc string) *Type {
	return b.register(name, inst, doc, ReferenceKind, NewRefType)
}

func (b Builder) RegisterValue(name string, inst any, doc string) *Type {
	return b.register(name, inst, doc, ValueKind, NewValueType)
}

func (b Builder) RegisterInterface(name string, inst any, doc string) *Type {
	return b.register(name, inst, doc, InterfaceKind, NewInterfaceType)
}

func (b Builder) register(name string, inst any, doc string, kind Kind, makeType func(string, any, any) *Type) *Type {
	var meta any
	if b.MetaFactory != nil {
		meta = b.MetaFactory(kind, name, doc)
	}
	key := &name
	if b.Intern != nil {
		key = b.Intern(name)
	}
	return b.Registry.Register(key, makeType(name, inst, meta))
}
