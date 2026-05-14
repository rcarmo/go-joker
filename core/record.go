package core

// record.go — Record support for Clojure parity.
//
// A Record is a named, typed map with fixed fields plus optional extension fields.
// Records support:
// - Keyword access: (:field record)
// - get/assoc/dissoc (dissoc to extension fields only; dissoc of base field returns plain map)
// - Equality by type + fields
// - Protocol satisfaction via extend-type with the record's type name

import (
	"fmt"
	"strings"
)

// RecordType describes a record class created by defrecord.
type RecordType struct {
	name     string
	fields   []string       // ordered base field names
	fieldIdx map[string]int // field name → index in bases
}

// Record is an instance of a RecordType.
type Record struct {
	InfoHolder
	MetaHolder
	rtype *RecordType
	bases []Object  // values for base fields (same order as rtype.fields)
	ext   *ArrayMap // extension fields (nil if none)
}

func (r *Record) ToString(escape bool) string {
	var b strings.Builder
	b.WriteString("#")
	b.WriteString(r.rtype.name)
	b.WriteString("{")
	first := true
	for i, fname := range r.rtype.fields {
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

func (r *Record) GetType() *Type { return TYPE.ArrayMap }
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

func (r *Record) WithInfo(info *ObjectInfo) Object {
	res := r.clone()
	res.info = info
	return res
}

func (r *Record) WithMeta(m Map) Object {
	res := r.clone()
	res.meta = SafeMerge(res.meta, m)
	return res
}

func (r *Record) clone() *Record {
	bases := make([]Object, len(r.bases))
	copy(bases, r.bases)
	var ext *ArrayMap
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

// --- Map interface ---

// Get implements Gettable for keyword access.
func (r *Record) Get(key Object) (bool, Object) {
	if kw, ok := key.(Keyword); ok {
		name := kw.ToString(false)[1:] // strip leading ":"
		if idx, ok := r.rtype.fieldIdx[name]; ok {
			return true, r.bases[idx]
		}
	}
	if r.ext != nil {
		return r.ext.Get(key)
	}
	return false, nil
}

// EntryAt returns a MapEntry for the given key.
func (r *Record) EntryAt(key Object) *ArrayVector {
	if ok, v := r.Get(key); ok {
		av := collections.EmptyArrayVector().Conj(key).(*ArrayVector).Conj(v).(*ArrayVector)
		return av
	}
	return nil
}

// Assoc returns a new record with the key set to val.
// If key is a base field, returns a new record. Otherwise extends.
func (r *Record) Assoc(key, val Object) Associative {
	if kw, ok := key.(Keyword); ok {
		name := kw.ToString(false)[1:]
		if idx, ok := r.rtype.fieldIdx[name]; ok {
			res := r.clone()
			res.bases[idx] = val
			return res
		}
	}
	res := r.clone()
	if res.ext == nil {
		res.ext = collections.EmptyArrayMap()
	}
	res.ext = res.ext.Assoc(key, val).(*ArrayMap)
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

// Seq returns a sequence of MapEntry pairs.
func (r *Record) Seq() Seq {
	entries := make([]Object, 0, r.Count())
	for i, fname := range r.rtype.fields {
		entries = append(entries, collections.VectorFrom(MakeKeyword(fname), r.bases[i]))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			entries = append(entries, collections.VectorFrom(p.Key, p.Value))
		}
	}
	return &ArraySeq{arr: entries, index: 0}
}

// Keys returns all keys.
func (r *Record) Keys() Seq {
	keys := make([]Object, 0, r.Count())
	for _, fname := range r.rtype.fields {
		keys = append(keys, MakeKeyword(fname))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			keys = append(keys, p.Key)
		}
	}
	return &ArraySeq{arr: keys, index: 0}
}

// Vals returns all values.
func (r *Record) Vals() Seq {
	vals := make([]Object, 0, r.Count())
	vals = append(vals, r.bases...)
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			vals = append(vals, p.Value)
		}
	}
	return &ArraySeq{arr: vals, index: 0}
}

// Conj adds a map entry to the record.
func (r *Record) Conj(obj Object) Conjable {
	switch v := obj.(type) {
	case *Vector:
		if v.count != 2 {
			panic(RT.NewError("Vector arg to conj on record must be a pair"))
		}
		return r.Assoc(v.at(0), v.at(1)).(Conjable)
	}
	panic(RT.NewError(fmt.Sprintf("Cannot conj %s onto record", obj.GetType().ToString(false))))
}

// Call implements keyword-style access: (record :field)
func (r *Record) Call(args []Object) Object {
	CheckArity(args, 1, 2)
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
func (r *Record) Merge(other Map) Map {
	res := r.clone()
	for iter := other.Iter(); iter.HasNext(); {
		p := iter.Next()
		assocResult := res.Assoc(p.Key, p.Value)
		res = assocResult.(*Record)
	}
	return res
}

// Iter returns a map iterator.
func (r *Record) Iter() MapIterator {
	return &recordIterator{r: r, idx: 0}
}

// Containskey
func (r *Record) ContainsKey(key Object) bool {
	ok, _ := r.Get(key)
	return ok
}

// Without (dissoc) — dissoc of a base field returns a plain map
func (r *Record) Without(key Object) Map {
	if kw, ok := key.(Keyword); ok {
		name := kw.ToString(false)[1:]
		if _, ok := r.rtype.fieldIdx[name]; ok {
			// Dissoc base field → degrade to plain map
			m := collections.EmptyArrayMap()
			for i, fname := range r.rtype.fields {
				if fname != name {
					m.Add(MakeKeyword(fname), r.bases[i])
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
		res.ext = res.ext.Without(key).(*ArrayMap)
		return res
	}
	return r
}

type recordIterator struct {
	r       *Record
	idx     int
	extIter MapIterator
}

func (it *recordIterator) HasNext() bool {
	if it.idx < len(it.r.rtype.fields) {
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

func (it *recordIterator) Next() *Pair {
	if it.idx < len(it.r.rtype.fields) {
		p := &Pair{
			Key:   MakeKeyword(it.r.rtype.fields[it.idx]),
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
func NewRecord(rtype *RecordType, fields []Object) *Record {
	if len(fields) != len(rtype.fields) {
		panic(RT.NewError(fmt.Sprintf("Wrong number of fields for record %s: expected %d, got %d",
			rtype.name, len(rtype.fields), len(fields))))
	}
	bases := make([]Object, len(fields))
	copy(bases, fields)
	return &Record{rtype: rtype, bases: bases}
}

// MakeRecordType creates a new record type descriptor.
func MakeRecordType(name string, fields []string) *RecordType {
	idx := make(map[string]int)
	for i, f := range fields {
		idx[f] = i
	}
	return &RecordType{name: name, fields: fields, fieldIdx: idx}
}
