package types

import (
	"fmt"

	"github.com/rcarmo/go-joker/core/hashutil"
	corestr "github.com/rcarmo/go-joker/core/string"
)

const KeywordHashMask uint32 = 0x7334c790

type Keyword struct {
	InfoHolder
	ns   *string
	name *string
	hash uint32
}

type Symbol struct {
	InfoHolder
	MetaHolder
	ns   *string
	name *string
	hash uint32
}

var NamedLookup func(Object, []Object) Object

func MakeSymbol(intern func(string) *string, nsname string) Symbol {
	ns, local, ok := corestr.SplitQualified(nsname)
	if !ok {
		return MakeSymbolFromKeys(nil, intern(local))
	}
	return MakeSymbolFromKeys(intern(ns), intern(local))
}

func MakeSymbolFromKeys(ns, name *string) Symbol {
	return Symbol{
		ns:   ns,
		name: name,
	}
}

func MakeKeyword(intern func(string) *string, nsname string) Keyword {
	ns, local, ok := corestr.SplitQualified(nsname)
	if !ok {
		return MakeKeywordFromKeys(nil, intern(local))
	}
	return MakeKeywordFromKeys(intern(ns), intern(local))
}

func MakeKeywordFromKeys(ns, name *string) Keyword {
	return Keyword{
		ns:   ns,
		name: name,
		hash: hashutil.Symbol(ns, name) ^ KeywordHashMask,
	}
}

func (k Keyword) ToString(escape bool) string {
	if k.ns != nil {
		return ":" + *k.ns + "/" + *k.name
	}
	return ":" + *k.name
}

func (k Keyword) Name() string          { return *k.name }
func (k Keyword) NameKey() *string      { return k.name }
func (k Keyword) NamespaceKey() *string { return k.ns }
func (k Keyword) Namespace() string {
	if k.ns != nil {
		return *k.ns
	}
	return ""
}

func (k Keyword) Equals(other interface{}) bool {
	switch other := other.(type) {
	case Keyword:
		return k.ns == other.ns && k.name == other.name
	default:
		return false
	}
}

func (k Keyword) GetType() *Type                   { return RuntimeTypes.Keyword }
func (k Keyword) WithInfo(info *ObjectInfo) Object { k.Info = info; return k }
func (k Keyword) Hash() uint32                     { return k.hash }
func (k Keyword) Compare(other Object) int {
	k2 := other.(Keyword)
	return corestr.Compare(k.ToString(false), k2.ToString(false))
}
func (k Keyword) Call(args []Object) Object { return namedLookup(k, args) }
func (k Keyword) AsGo() string {
	if k.NameKey() == nil {
		panic("empty keyword")
	}
	return "kw_" + corestr.KeywordGoName(k.ToString(false)) + infoSuffix(k.Info)
}

func (s Symbol) ToString(escape bool) string {
	if s.ns != nil {
		return *s.ns + "/" + *s.name
	}
	return *s.name
}

func (s Symbol) Name() string          { return *s.name }
func (s Symbol) NameKey() *string      { return s.name }
func (s Symbol) NamespaceKey() *string { return s.ns }
func (s Symbol) Namespace() string {
	if s.ns != nil {
		return *s.ns
	}
	return ""
}

func (s Symbol) Equals(other interface{}) bool {
	switch other := other.(type) {
	case Symbol:
		return s.ns == other.ns && s.name == other.name
	default:
		return false
	}
}

func (s Symbol) GetType() *Type                    { return RuntimeTypes.Symbol }
func (s Symbol) WithInfo(info *ObjectInfo) Object  { s.Info = info; return s }
func (s Symbol) Hash() uint32                      { return hashutil.Symbol(s.ns, s.name) + 0x9e3779b9 }
func (s Symbol) PackedHash() uint32                { return s.hash }
func (s Symbol) WithPackedHash(hash uint32) Symbol { s.hash = hash; return s }
func (s Symbol) WithMeta(meta Map) Object {
	res := s
	res.Meta = SafeMerge(res.Meta, meta)
	return res
}
func (s Symbol) Compare(other Object) int {
	s2 := other.(Symbol)
	return corestr.Compare(s.ToString(false), s2.ToString(false))
}
func (s Symbol) Call(args []Object) Object { return namedLookup(s, args) }
func (s Symbol) AsGo() string {
	name := "EMPTY"
	if s.NameKey() != nil {
		name = corestr.SymbolGoName(s.ToString(false))
	}
	return "symbol_" + name + infoSuffix(s.Info)
}

func infoSuffix(info *ObjectInfo) string {
	if info == nil || (info.EndLine == 0 && info.EndColumn == 0 && info.StartLine == 0 && info.StartColumn == 0 && (info.Filename == nil || *info.Filename == "")) {
		return ""
	}
	filename := ""
	if info.Filename != nil {
		filename = corestr.GoName(corestr.FilenameUnbracketed(*info.Filename))
		if filename != "" && filename != "_" {
			filename += "_"
		}
	}
	return fmt.Sprintf("_POS_%s%d_%d__%d_%d", filename, info.StartLine, info.StartColumn, info.EndLine, info.EndColumn)
}

func namedLookup(key Object, args []Object) Object {
	if NamedLookup == nil {
		panic("named lookup callback is not installed")
	}
	return NamedLookup(key, args)
}
