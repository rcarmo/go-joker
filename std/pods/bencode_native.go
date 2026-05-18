package pods

import (
	"bytes"
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	"github.com/zeebo/bencode"

	. "github.com/rcarmo/go-joker/core"
)

func bencodeEncodeObject(obj coretypes.Object) []byte {
	return bencodeEncodePlain(objectToBencode(obj))
}

func bencodeEncodePlain(v interface{}) []byte {
	var buf bytes.Buffer
	if err := bencode.NewEncoder(&buf).Encode(v); err != nil {
		panic(RT.NewError("pods/bencode-encode: " + err.Error()))
	}
	return buf.Bytes()
}

func bencodeDecodeBytes(data []byte) coretypes.Object {
	var v interface{}
	if err := bencode.NewDecoder(bytes.NewReader(data)).Decode(&v); err != nil {
		panic(RT.NewError("pods/bencode-decode: " + err.Error()))
	}
	return bencodeToObject(v)
}

func bencodeDecodeReader(r io.Reader) (coretypes.Object, error) {
	var v interface{}
	if err := bencode.NewDecoder(r).Decode(&v); err != nil {
		return NIL, err
	}
	return bencodeToObject(v), nil
}

func objectToBencode(obj coretypes.Object) interface{} {
	switch v := obj.(type) {
	case Nil:
		return ""
	case coretypes.String:
		return v.S
	case coretypes.Keyword, coretypes.Symbol:
		return v.ToString(false)
	case coretypes.Int:
		return int64(v.I)
	case *coretypes.BigInt:
		return v.BigInt().String()
	case coretypes.Boolean:
		if v.B {
			return int64(1)
		}
		return int64(0)
	case coretypes.Map:
		m := make(map[string]interface{})
		for it := v.Iter(); it.HasNext(); {
			p := it.Next()
			m[bencodeKeyString(p.Key)] = objectToBencode(p.Value)
		}
		return m
	case coretypes.Seqable:
		arr := []interface{}{}
		for s := v.Seq(); !s.IsEmpty(); s = s.Rest() {
			arr = append(arr, objectToBencode(s.First()))
		}
		return arr
	default:
		return v.ToString(false)
	}
}

func bencodeKeyString(k coretypes.Object) string {
	switch v := k.(type) {
	case coretypes.String:
		return v.S
	case coretypes.Keyword:
		return v.ToString(false)[1:]
	case coretypes.Symbol:
		return v.ToString(false)
	default:
		return v.ToString(false)
	}
}

func bencodeToObject(v interface{}) coretypes.Object {
	switch x := v.(type) {
	case string:
		return coretypes.MakeString(x)
	case []byte:
		return coretypes.MakeString(string(x))
	case int:
		return coretypes.MakeInt(x)
	case int64:
		return coretypes.MakeInt(int(x))
	case uint64:
		return coretypes.MakeInt(int(x))
	case []interface{}:
		objs := make([]coretypes.Object, len(x))
		for i, e := range x {
			objs[i] = bencodeToObject(e)
		}
		return NewVectorFrom(objs...)
	case map[string]interface{}:
		m := EmptyArrayMap()
		for k, val := range x {
			m.Add(coretypes.MakeString(k), bencodeToObject(val))
		}
		return m
	case map[interface{}]interface{}:
		m := EmptyArrayMap()
		for k, val := range x {
			m.Add(coretypes.MakeString(fmt.Sprint(k)), bencodeToObject(val))
		}
		return m
	default:
		return coretypes.MakeString(fmt.Sprint(v))
	}
}
