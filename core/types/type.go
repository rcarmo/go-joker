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

func (t *Type) ToString(escape bool) string   { return t.Name }
func (t *Type) Equals(other interface{}) bool { return t == other }
func (t *Type) GetInfo() *ObjectInfo          { return nil }
func (t *Type) GetType() *Type                { return t }
func (t *Type) WithInfo(*ObjectInfo) *Type    { return t }
func (t *Type) Hash() uint32                  { return uint32(uintptr(unsafe.Pointer(t))) }
