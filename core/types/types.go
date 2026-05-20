package types

import (
	"reflect"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

// Object is the root-independent read-only portion of the Joker object
// protocol. Root core still has a local Object alias while the remaining
// collection and runtime contracts are moved over incrementally.
type Object interface {
	Equality
	ToString(escape bool) string
	GetInfo() *ObjectInfo
	GetType() *Type
	Hash() uint32
}

type Meta interface {
	GetMeta() Map
	WithMeta(Map) Object
}

type MetaHolder struct {
	Meta Map
}

func (m MetaHolder) GetMeta() Map      { return m.Meta }
func (m *MetaHolder) SetMeta(meta Map) { m.Meta = meta }

type Ref interface {
	AlterMeta(fn Callable, args []Object) Map
	ResetMeta(m Map) Map
}

type Set interface {
	Conjable
	Gettable
	Disjoin(key Object) Set
}

type Vec interface {
	Object
	CountedIndexed
	Gettable
	Associative
	Sequential
	Comparable
	Indexed
	Stack
	Reversible
	Meta
	Seqable
	Formatter
	Callable
}

type Char struct {
	InfoHolder
	Ch rune
}

func MakeChar(r rune) Char { return Char{Ch: r} }

func (c Char) ToString(escape bool) string   { return corestr.CharString(c.Ch, escape) }
func (c Char) Equals(other interface{}) bool { o, ok := other.(Char); return ok && c.Ch == o.Ch }
func (c Char) GetType() *Type                { return RuntimeTypes.Char }
func (c Char) Native() interface{}           { return c.Ch }
func (c Char) Hash() uint32                  { h := hashutil.New32(); h.Write([]byte(string(c.Ch))); return h.Sum32() }
func (c Char) Compare(other Object) int {
	c2 := other.(Char)
	if c.Ch < c2.Ch {
		return -1
	}
	if c2.Ch < c.Ch {
		return 1
	}
	return 0
}

type Hash32 interface {
	Write([]byte) (int, error)
	Sum32() uint32
}

func NewHash32() Hash32 { return hashutil.New32() }

type Kind string

const (
	ReferenceKind Kind = "Concrete reference type"
	ValueKind     Kind = "Concrete type"
	InterfaceKind Kind = "Interface type"
)

func (k Kind) DocumentationPrefix() string { return "(" + string(k) + ")" }

// Type describes a Joker runtime type. Root core still owns registry population
// until bootstrap/proc systems move out.
type Type struct {
	MetaHolder
	Name        string
	ReflectType reflect.Type
}

func NewType(name string, reflectType reflect.Type, metaHolder any) *Type {
	t := &Type{Name: name, ReflectType: reflectType}
	if meta, ok := metaHolder.(Map); ok {
		t.Meta = meta
	}
	return t
}

func NewRefType(name string, inst any, metaHolder any) *Type {
	return NewType(name, reflect.TypeOf(inst), metaHolder)
}

func NewValueType(name string, inst any, metaHolder any) *Type {
	return NewType(name, reflect.TypeOf(inst).Elem(), metaHolder)
}

func NewInterfaceType(name string, inst any, metaHolder any) *Type {
	return NewType(name, reflect.TypeOf(inst).Elem(), metaHolder)
}

func IsEqualOrImplements(abstractType *Type, concreteType *Type) bool {
	if abstractType.ReflectType.Kind() == reflect.Interface {
		return concreteType.ReflectType.Implements(abstractType.ReflectType)
	}
	return concreteType.ReflectType == abstractType.ReflectType
}

func (t *Type) ToString(escape bool) string   { return t.Name }
func (t *Type) Equals(other interface{}) bool { return t == other }
func (t *Type) GetInfo() *ObjectInfo          { return nil }
func (t *Type) GetType() *Type                { return t }
func (t *Type) WithInfo(*ObjectInfo) *Type    { return t }
func (t *Type) WithMeta(meta Map) Object {
	res := *t
	res.Meta = SafeMerge(res.Meta, meta)
	return &res
}
func (t *Type) Hash() uint32 { return uint32(uintptr(unsafe.Pointer(t))) }
