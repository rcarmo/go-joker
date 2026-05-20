package core

// record.go — Record support for Clojure parity.
//
// A Record is a named, typed map with fixed fields plus optional extension fields.
// Records support:
// - Keyword access: (:field record)
// - get/assoc/dissoc (dissoc to extension fields only; dissoc of base field returns plain map)
// - coretypes.Equality by type + fields
// - Protocol satisfaction via extend-type with the record's type name

import (
	"fmt"
	"strings"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

// Record is an instance of a RecordType.
type Record struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	rtype *coretypes.RecordType
	bases []coretypes.Object        // values for base fields (same order as rtype.fields)
	ext   *corecollections.ArrayMap // extension fields (nil if none)
}

func (r *Record) ToString(escape bool) string {
	var b strings.Builder
	b.WriteString("#")
	b.WriteString(r.rtype.Name)
	b.WriteString("{")
	first := true
	for i, fname := range r.rtype.Fields {
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(":")
		b.WriteString(fname)
		b.WriteString(" ")
		b.WriteString(r.bases[i].ToString(escape))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			if !first {
				b.WriteString(", ")
			}
			first = false
			b.WriteString(p.Key.ToString(escape))
			b.WriteString(" ")
			b.WriteString(p.Value.ToString(escape))
		}
	}
	b.WriteString("}")
	return b.String()
}

func (r *Record) Equals(other interface{}) bool {
	o, ok := other.(*Record)
	if !ok {
		return false
	}
	if r.rtype != o.rtype {
		return false
	}
	for i := range r.bases {
		if !r.bases[i].Equals(o.bases[i]) {
			return false
		}
	}
	// Compare extension fields
	if r.ext == nil && o.ext == nil {
		return true
	}
	if r.ext == nil || o.ext == nil {
		rCount := 0
		oCount := 0
		if r.ext != nil {
			rCount = r.ext.Count()
		}
		if o.ext != nil {
			oCount = o.ext.Count()
		}
		return rCount == 0 && oCount == 0
	}
	return r.ext.Equals(o.ext)
}

func (r *Record) GetType() *coretypes.Type { return TYPE.ArrayMap }
func (r *Record) Hash() uint32 {
	h := uint32(0x9e3779b9)
	for _, v := range r.bases {
		h = h*31 + v.Hash()
	}
	if r.ext != nil {
		h = h*31 + r.ext.Hash()
	}
	return h
}

func (r *Record) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := r.clone()
	res.Info = info
	return res
}

func (r *Record) WithMeta(m coretypes.Map) coretypes.Object {
	res := r.clone()
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return res
}

func (r *Record) clone() *Record {
	bases := make([]coretypes.Object, len(r.bases))
	copy(bases, r.bases)
	var ext *corecollections.ArrayMap
	if r.ext != nil {
		ext = r.ext.Clone()
	}
	return &Record{
		InfoHolder: r.InfoHolder,
		MetaHolder: r.MetaHolder,
		rtype:      r.rtype,
		bases:      bases,
		ext:        ext,
	}
}

// --- coretypes.Map interface ---

// Get implements coretypes.Gettable for keyword access.
func (r *Record) Get(key coretypes.Object) (bool, coretypes.Object) {
	if kw, ok := key.(coretypes.Keyword); ok {
		name := kw.ToString(false)[1:] // strip leading ":"
		if idx, ok := r.rtype.FieldIdx[name]; ok {
			return true, r.bases[idx]
		}
	}
	if r.ext != nil {
		return r.ext.Get(key)
	}
	return false, nil
}

// EntryAt returns a MapEntry for the given key.
func (r *Record) EntryAt(key coretypes.Object) coretypes.Object {
	if ok, v := r.Get(key); ok {
		av := corecollections.EmptyArrayVector().Conj(key).(*corecollections.ArrayVector).Conj(v).(*corecollections.ArrayVector)
		return av
	}
	return nil
}

// Assoc returns a new record with the key set to val.
// If key is a base field, returns a new record. Otherwise extends.
func (r *Record) Assoc(key, val coretypes.Object) coretypes.Associative {
	if kw, ok := key.(coretypes.Keyword); ok {
		name := kw.ToString(false)[1:]
		if idx, ok := r.rtype.FieldIdx[name]; ok {
			res := r.clone()
			res.bases[idx] = val
			return res
		}
	}
	res := r.clone()
	if res.ext == nil {
		res.ext = corecollections.EmptyArrayMap()
	}
	res.ext = res.ext.Assoc(key, val).(*corecollections.ArrayMap)
	return res
}

// Count returns the number of fields (base + extension).
func (r *Record) Count() int {
	n := len(r.bases)
	if r.ext != nil {
		n += r.ext.Count()
	}
	return n
}

// coretypes.Seq returns a sequence of MapEntry pairs.
func (r *Record) Seq() coretypes.Seq {
	entries := make([]coretypes.Object, 0, r.Count())
	for i, fname := range r.rtype.Fields {
		entries = append(entries, corecollections.NewVectorFrom(coretypes.MakeKeyword(STRINGS.Intern, fname), r.bases[i]))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			entries = append(entries, corecollections.NewVectorFrom(p.Key, p.Value))
		}
	}
	return &corecollections.ArraySeq{Arr: entries, Index: 0}
}

// Keys returns all keys.
func (r *Record) Keys() coretypes.Seq {
	keys := make([]coretypes.Object, 0, r.Count())
	for _, fname := range r.rtype.Fields {
		keys = append(keys, coretypes.MakeKeyword(STRINGS.Intern, fname))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			keys = append(keys, p.Key)
		}
	}
	return &corecollections.ArraySeq{Arr: keys, Index: 0}
}

// Vals returns all values.
func (r *Record) Vals() coretypes.Seq {
	vals := make([]coretypes.Object, 0, r.Count())
	vals = append(vals, r.bases...)
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			vals = append(vals, p.Value)
		}
	}
	return &corecollections.ArraySeq{Arr: vals, Index: 0}
}

// Conj adds a map entry to the record.
func (r *Record) Conj(obj coretypes.Object) coretypes.Conjable {
	switch v := obj.(type) {
	case *corecollections.Vector:
		if v.Count() != 2 {
			panic(coretypes.RuntimeError("corecollections.Vector arg to conj on record must be a pair"))
		}
		return r.Assoc(v.At(0), v.At(1)).(coretypes.Conjable)
	}
	panic(coretypes.RuntimeError(fmt.Sprintf("Cannot conj %s onto record", obj.GetType().ToString(false))))
}

// Call implements keyword-style access: (record :field)
func (r *Record) Call(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 2)
	ok, v := r.Get(args[0])
	if ok {
		return v
	}
	if len(args) == 2 {
		return args[1]
	}
	return NIL
}

// Merge merges a map into the record.
func (r *Record) Merge(other coretypes.Map) coretypes.Map {
	res := r.clone()
	for iter := other.Iter(); iter.HasNext(); {
		p := iter.Next()
		assocResult := res.Assoc(p.Key, p.Value)
		res = assocResult.(*Record)
	}
	return res
}

// Iter returns a map iterator.
func (r *Record) Iter() coretypes.MapIterator {
	return &recordIterator{r: r, idx: 0}
}

// Containskey
func (r *Record) ContainsKey(key coretypes.Object) bool {
	ok, _ := r.Get(key)
	return ok
}

// Without (dissoc) — dissoc of a base field returns a plain map
func (r *Record) Without(key coretypes.Object) coretypes.Map {
	if kw, ok := key.(coretypes.Keyword); ok {
		name := kw.ToString(false)[1:]
		if _, ok := r.rtype.FieldIdx[name]; ok {
			// Dissoc base field → degrade to plain map
			m := corecollections.EmptyArrayMap()
			for i, fname := range r.rtype.Fields {
				if fname != name {
					m.Add(coretypes.MakeKeyword(STRINGS.Intern, fname), r.bases[i])
				}
			}
			if r.ext != nil {
				for iter := r.ext.Iter(); iter.HasNext(); {
					p := iter.Next()
					m.Add(p.Key, p.Value)
				}
			}
			return m
		}
	}
	if r.ext != nil {
		res := r.clone()
		res.ext = res.ext.Without(key).(*corecollections.ArrayMap)
		return res
	}
	return r
}

type recordIterator struct {
	r       *Record
	idx     int
	extIter coretypes.MapIterator
}

func (it *recordIterator) HasNext() bool {
	if it.idx < len(it.r.rtype.Fields) {
		return true
	}
	if it.r.ext != nil {
		if it.extIter == nil {
			it.extIter = it.r.ext.Iter()
		}
		return it.extIter.HasNext()
	}
	return false
}

func (it *recordIterator) Next() *coretypes.Pair {
	if it.idx < len(it.r.rtype.Fields) {
		p := &coretypes.Pair{
			Key:   coretypes.MakeKeyword(STRINGS.Intern, it.r.rtype.Fields[it.idx]),
			Value: it.r.bases[it.idx],
		}
		it.idx++
		return p
	}
	if it.extIter == nil {
		it.extIter = it.r.ext.Iter()
	}
	return it.extIter.Next()
}

// NewRecord creates a new record instance.
func NewRecord(rtype *coretypes.RecordType, fields []coretypes.Object) *Record {
	if len(fields) != len(rtype.Fields) {
		panic(coretypes.RuntimeError(fmt.Sprintf("Wrong number of fields for record %s: expected %d, got %d",
			rtype.Name, len(rtype.Fields), len(fields))))
	}
	bases := make([]coretypes.Object, len(fields))
	copy(bases, fields)
	return &Record{rtype: rtype, bases: bases}
}
