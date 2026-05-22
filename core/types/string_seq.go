package types

import (
	"fmt"
	"unicode/utf8"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

type StringSeq struct {
	InfoHolder
	S   string
	Off int
}

func (s String) Seq() Seq { return &StringSeq{S: s.S} }

func (s *StringSeq) Seq() Seq { return s }

func (s *StringSeq) ToString(escape bool) string {
	return fmt.Sprintf("#object[StringSeq %d/%d]", s.Off, utf8.RuneCountInString(s.S))
}
func (s *StringSeq) Equals(other interface{}) bool {
	o, ok := other.(*StringSeq)
	return ok && s.S == o.S && s.Off == o.Off
}
func (s *StringSeq) GetType() *Type { return RuntimeTypes.StringSeq }
func (s *StringSeq) Hash() uint32 {
	h := hashutil.New32()
	h.Write([]byte(s.S))
	h.Write([]byte(fmt.Sprintf(":%d", s.Off)))
	return h.Sum32()
}
func (s *StringSeq) WithInfo(info *ObjectInfo) Object {
	res := *s
	res.Info = info
	return &res
}
func (s *StringSeq) IsEmpty() bool { return s.Off >= utf8.RuneCountInString(s.S) }
func (s *StringSeq) First() Object {
	if s.IsEmpty() {
		return RuntimeNil
	}
	r, _, _ := corestr.NthRune(s.S, s.Off)
	return Char{Ch: r}
}
func (s *StringSeq) Rest() Seq {
	if s.IsEmpty() {
		return s
	}
	return &StringSeq{S: s.S, Off: s.Off + 1}
}
func (s *StringSeq) Cons(obj Object) Seq { return &StringConsSeq{FirstValue: obj, RestValue: s} }

type StringConsSeq struct {
	InfoHolder
	FirstValue Object
	RestValue  Seq
}

func (s *StringConsSeq) Seq() Seq                         { return s }
func (s *StringConsSeq) ToString(escape bool) string      { return "#object[StringConsSeq]" }
func (s *StringConsSeq) Equals(other interface{}) bool    { return s == other }
func (s *StringConsSeq) GetType() *Type                   { return RuntimeTypes.Seq }
func (s *StringConsSeq) Hash() uint32                     { return hashutil.Ptr(uintptr(unsafe.Pointer(s))) }
func (s *StringConsSeq) WithInfo(info *ObjectInfo) Object { res := *s; res.Info = info; return &res }
func (s *StringConsSeq) IsEmpty() bool                    { return false }
func (s *StringConsSeq) First() Object                    { return s.FirstValue }
func (s *StringConsSeq) Rest() Seq                        { return s.RestValue }
func (s *StringConsSeq) Cons(obj Object) Seq              { return &StringConsSeq{FirstValue: obj, RestValue: s} }
