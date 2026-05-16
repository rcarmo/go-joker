package transit

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	. "github.com/rcarmo/go-joker/core"
)

const (
	transitCacheDigits     = 44
	transitCacheBase       = 48
	transitCacheSize       = transitCacheDigits * transitCacheDigits
	transitMinCacheableLen = 4
	transitCacheMarker     = "^"
	transitMapTag          = "^ "
)

type transitCache struct {
	keyToVal map[string]string
	valToKey map[string]string
	idx      int
}

type transitEncoder struct{ cache *transitCache }
type transitDecoder struct{ cache *transitCache }

func newTransitCache() *transitCache {
	return &transitCache{keyToVal: make(map[string]string), valToKey: make(map[string]string)}
}

func (c *transitCache) encodeKey(idx int) string {
	hi := idx / transitCacheDigits
	lo := idx % transitCacheDigits
	if hi == 0 {
		return transitCacheMarker + string(rune(lo+transitCacheBase))
	}
	return transitCacheMarker + string(rune(hi+transitCacheBase)) + string(rune(lo+transitCacheBase))
}

func transitIsCacheRef(s string) bool { return len(s) > 0 && s[0] == '^' && s != transitMapTag }

func (c *transitCache) isCacheable(s string, asKey bool) bool {
	if len(s) < transitMinCacheableLen {
		return false
	}
	if asKey {
		return true
	}
	return len(s) >= 2 && s[0] == '~' && (s[1] == '#' || s[1] == ':' || s[1] == '$')
}

func (c *transitCache) remember(s string, asKey bool) {
	if !c.isCacheable(s, asKey) {
		return
	}
	if len(c.keyToVal) >= transitCacheSize {
		c.keyToVal = make(map[string]string)
		c.valToKey = make(map[string]string)
		c.idx = 0
	}
	key := c.encodeKey(c.idx)
	c.keyToVal[key] = s
	c.valToKey[s] = key
	c.idx++
}

func (c *transitCache) read(s string, asKey bool) string {
	if transitIsCacheRef(s) {
		if v, ok := c.keyToVal[s]; ok {
			return v
		}
		return s
	}
	c.remember(s, asKey)
	return s
}

func (c *transitCache) write(s string, asKey bool) string {
	if ref, ok := c.valToKey[s]; ok {
		return ref
	}
	c.remember(s, asKey)
	return s
}

func writeTransit(obj Object) Object { return MakeString(writeTransitString(obj, false)) }

func writeTransitVerbose(obj Object) Object { return MakeString(writeTransitString(obj, true)) }

func writeTransitString(obj Object, verbose bool) string {
	enc := &transitEncoder{cache: newTransitCache()}
	if verbose {
		enc.cache = nil
	}
	v := enc.encode(obj, false)
	b, err := json.Marshal(v)
	if err != nil {
		panic(RT.NewError("transit/write: " + err.Error()))
	}
	return string(b)
}

func readTransit(s String) Object {
	var v interface{}
	if err := json.Unmarshal([]byte(s.S), &v); err != nil {
		panic(RT.NewError("transit/read: " + err.Error()))
	}
	dec := &transitDecoder{cache: newTransitCache()}
	return dec.decode(v, false)
}

func (e *transitEncoder) encode(obj Object, asKey bool) interface{} {
	switch v := obj.(type) {
	case Nil:
		return nil
	case Boolean:
		return v.B
	case Int:
		return v.I
	case Double:
		return v.D
	case *BigInt:
		return e.cacheString("~i"+v.BigInt().String(), asKey)
	case *BigFloat:
		return e.cacheString("~f"+v.BigFloat().Text('g', -1), asKey)
	case *Ratio:
		return e.cacheString("~r"+v.Ratio().String(), asKey)
	case String:
		return e.cacheString(transitEncodeString(v.S), asKey)
	case Keyword:
		return e.cacheString("~:"+strings.TrimPrefix(v.ToString(false), ":"), asKey)
	case Symbol:
		return e.cacheString("~$"+v.ToString(false), asKey)
	}
	if set, ok := obj.(*MapSet); ok {
		items := []interface{}{}
		for s := set.Seq(); !s.IsEmpty(); s = s.Rest() {
			items = append(items, e.encode(s.First(), false))
		}
		return []interface{}{e.cacheString("~#set", false), items}
	}
	if m, ok := obj.(Map); ok {
		arr := []interface{}{transitMapTag}
		for it := m.Iter(); it.HasNext(); {
			p := it.Next()
			arr = append(arr, e.encode(p.Key, true), e.encode(p.Value, false))
		}
		return arr
	}
	if s, ok := obj.(Seqable); ok {
		arr := []interface{}{}
		seq := s.Seq()
		for !seq.IsEmpty() {
			arr = append(arr, e.encode(seq.First(), false))
			seq = seq.Rest()
		}
		return arr
	}
	return e.cacheString(transitEncodeString(obj.ToString(false)), asKey)
}

func (e *transitEncoder) cacheString(s string, asKey bool) string {
	if e.cache == nil {
		return s
	}
	return e.cache.write(s, asKey)
}

func transitEncodeString(s string) string {
	if strings.HasPrefix(s, "~") || s == transitMapTag || transitIsCacheRef(s) {
		return "~" + s
	}
	return s
}

func (d *transitDecoder) decode(v interface{}, asKey bool) Object {
	switch x := v.(type) {
	case nil:
		return NIL
	case bool:
		return MakeBoolean(x)
	case float64:
		if x == float64(int(x)) {
			return MakeInt(int(x))
		}
		return Double{D: x}
	case string:
		return d.decodeString(x, asKey)
	case []interface{}:
		return d.decodeArray(x)
	case map[string]interface{}:
		m := EmptyArrayMap()
		for k, val := range x {
			m.Add(MakeString(k), d.decode(val, false))
		}
		return m
	default:
		panic(RT.NewError(fmt.Sprintf("transit/read: unsupported JSON value %T", v)))
	}
}

func (d *transitDecoder) decodeString(s string, asKey bool) Object {
	if d.cache != nil {
		s = d.cache.read(s, asKey)
	}
	return transitDecodeString(s)
}

func (d *transitDecoder) decodeArray(x []interface{}) Object {
	if len(x) > 0 {
		if tag, ok := x[0].(string); ok {
			resolved := tag
			if d.cache != nil {
				resolved = d.cache.read(tag, false)
			}
			if resolved == transitMapTag {
				if (len(x)-1)%2 != 0 {
					panic(RT.NewError("transit/read: map array has odd number of entries"))
				}
				m := EmptyArrayMap()
				for i := 1; i < len(x); i += 2 {
					m.Add(d.decode(x[i], true), d.decode(x[i+1], false))
				}
				return m
			}
			if strings.HasPrefix(resolved, "~#") && len(x) == 2 {
				return d.decodeTagged(resolved[2:], x[1])
			}
		}
	}
	objs := make([]Object, len(x))
	for i, e := range x {
		objs[i] = d.decode(e, false)
	}
	return NewVectorFrom(objs...)
}

func (d *transitDecoder) decodeTagged(tag string, payload interface{}) Object {
	switch tag {
	case "set":
		items, ok := payload.([]interface{})
		if !ok {
			break
		}
		set := EmptySet()
		for _, item := range items {
			set.Add(d.decode(item, false))
		}
		return set
	case "list":
		items, ok := payload.([]interface{})
		if !ok {
			break
		}
		objs := make([]Object, len(items))
		for i, item := range items {
			objs[i] = d.decode(item, false)
		}
		return NewListFrom(objs...)
	case "'":
		return d.decode(payload, false)
	case "cmap":
		items, ok := payload.([]interface{})
		if !ok {
			break
		}
		if len(items)%2 != 0 {
			panic(RT.NewError("transit/read: cmap has odd number of entries"))
		}
		m := EmptyArrayMap()
		for i := 0; i < len(items); i += 2 {
			m.Add(d.decode(items[i], false), d.decode(items[i+1], false))
		}
		return m
	}
	return d.decode(payload, false)
}

func transitDecodeString(s string) Object {
	if strings.HasPrefix(s, "~~") {
		return MakeString(s[1:])
	}
	if strings.HasPrefix(s, "~:") {
		return MakeKeyword(s[2:])
	}
	if strings.HasPrefix(s, "~$") {
		return MakeSymbol(s[2:])
	}
	if strings.HasPrefix(s, "~i") {
		if bi, ok := new(big.Int).SetString(s[2:], 10); ok {
			if bi.IsInt64() {
				return MakeInt(int(bi.Int64()))
			}
			return MakeBigInt(bi)
		}
	}
	if strings.HasPrefix(s, "~f") {
		if bf, ok := MakeBigFloatWithOrig(s[2:], ""); ok {
			return bf
		}
	}
	if strings.HasPrefix(s, "~r") {
		if r, ok := new(big.Rat).SetString(s[2:]); ok {
			return MakeRatio(r)
		}
	}
	return MakeString(s)
}
