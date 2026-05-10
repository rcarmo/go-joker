package transit

import (
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/candid82/joker/core"
)

const transitMapTag = "^ "

func writeTransit(obj Object) Object {
	v := transitEncode(obj)
	b, err := json.Marshal(v)
	if err != nil {
		panic(RT.NewError("transit/write: " + err.Error()))
	}
	return MakeString(string(b))
}

func readTransit(s String) Object {
	var v interface{}
	if err := json.Unmarshal([]byte(s.S), &v); err != nil {
		panic(RT.NewError("transit/read: " + err.Error()))
	}
	return transitDecode(v)
}

func transitEncode(obj Object) interface{} {
	switch v := obj.(type) {
	case Nil:
		return nil
	case Boolean:
		return v.B
	case Int:
		return v.I
	case Double:
		return v.D
	case String:
		return transitEncodeString(v.S)
	case Keyword:
		return "~:" + strings.TrimPrefix(v.ToString(false), ":")
	case Symbol:
		return "~$" + v.ToString(false)
	case Seqable:
		// Maps also implement Seqable, so handle Map first below via type switch.
	}
	if m, ok := obj.(Map); ok {
		arr := []interface{}{transitMapTag}
		for it := m.Iter(); it.HasNext(); {
			p := it.Next()
			arr = append(arr, transitEncode(p.Key), transitEncode(p.Value))
		}
		return arr
	}
	if s, ok := obj.(Seqable); ok {
		arr := []interface{}{}
		seq := s.Seq()
		for !seq.IsEmpty() {
			arr = append(arr, transitEncode(seq.First()))
			seq = seq.Rest()
		}
		return arr
	}
	return transitEncodeString(obj.ToString(false))
}

func transitEncodeString(s string) string {
	if strings.HasPrefix(s, "~") || s == transitMapTag {
		return "~" + s
	}
	return s
}

func transitDecode(v interface{}) Object {
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
		return transitDecodeString(x)
	case []interface{}:
		if len(x) > 0 {
			if tag, ok := x[0].(string); ok && tag == transitMapTag {
				if (len(x)-1)%2 != 0 {
					panic(RT.NewError("transit/read: map array has odd number of entries"))
				}
				m := EmptyArrayMap()
				for i := 1; i < len(x); i += 2 {
					m.Add(transitDecode(x[i]), transitDecode(x[i+1]))
				}
				return m
			}
		}
		objs := make([]Object, len(x))
		for i, e := range x {
			objs[i] = transitDecode(e)
		}
		return NewVectorFrom(objs...)
	case map[string]interface{}:
		m := EmptyArrayMap()
		for k, val := range x {
			m.Add(MakeString(k), transitDecode(val))
		}
		return m
	default:
		panic(RT.NewError(fmt.Sprintf("transit/read: unsupported JSON value %T", v)))
	}
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
	return MakeString(s)
}
