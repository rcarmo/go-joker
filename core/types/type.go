package types

import (
	"reflect"
	"unsafe"
)

// Type describes a Joker runtime type. Root core still owns registry population
// until bootstrap/proc systems move out.
type Type struct {
	MetaHolder  any
	Name        string
	ReflectType reflect.Type
}

func NewType(name string, reflectType reflect.Type, metaHolder any) *Type {
	return &Type{MetaHolder: metaHolder, Name: name, ReflectType: reflectType}
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
func (t *Type) Hash() uint32                  { return uint32(uintptr(unsafe.Pointer(t))) }
