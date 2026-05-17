package pods

import (
	"encoding/json"
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"

	. "github.com/rcarmo/go-joker/core"
	edn "github.com/rcarmo/go-joker/std/edn"
	transit "github.com/rcarmo/go-joker/std/transit"
)

func (p *Pod) encodeArgs(args []coretypes.Object) (string, error) {
	switch p.format {
	case "", "json":
		vals := make([]interface{}, len(args))
		for i, a := range args {
			vals[i] = podPayloadFromObject(a)
		}
		bs, err := json.Marshal(vals)
		return string(bs), err
	case "edn":
		objs := make([]coretypes.Object, len(args))
		copy(objs, args)
		return edn.WriteEDNString(NewVectorFrom(objs...)), nil
	case "transit+json":
		return transit.TransitEncodeArgs(args)
	default:
		return "", fmt.Errorf("unsupported pod format: %s", p.format)
	}
}

func (p *Pod) decodePayload(s string) (coretypes.Object, error) {
	switch p.format {
	case "", "json":
		var v interface{}
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			return NIL, err
		}
		return podPayloadToObject(v), nil
	case "edn":
		return edn.ReadEDNString(s)
	case "transit+json":
		return transit.TransitDecodeValue(s)
	default:
		return NIL, fmt.Errorf("unsupported pod format: %s", p.format)
	}
}

func podPayloadFromObject(obj coretypes.Object) interface{} {
	switch v := obj.(type) {
	case Nil:
		return nil
	case coretypes.Boolean:
		return v.B
	case coretypes.Int:
		return v.I
	case coretypes.Double:
		return v.D
	case coretypes.String:
		return v.S
	case Keyword:
		return v.ToString(false)
	case Symbol:
		return v.ToString(false)
	case Map:
		m := make(map[string]interface{})
		for it := v.Iter(); it.HasNext(); {
			p := it.Next()
			m[bencodeKeyString(p.Key)] = podPayloadFromObject(p.Value)
		}
		return m
	case coretypes.Seqable:
		arr := []interface{}{}
		for s := v.Seq(); !s.IsEmpty(); s = s.Rest() {
			arr = append(arr, podPayloadFromObject(s.First()))
		}
		return arr
	default:
		return v.ToString(false)
	}
}

func podPayloadToObject(v interface{}) coretypes.Object {
	switch x := v.(type) {
	case nil:
		return NIL
	case bool:
		return coretypes.Boolean{B: x}
	case string:
		return coretypes.MakeString(x)
	case float64:
		if x == float64(int(x)) {
			return coretypes.MakeInt(int(x))
		}
		return coretypes.Double{D: x}
	case []interface{}:
		objs := make([]coretypes.Object, len(x))
		for i, e := range x {
			objs[i] = podPayloadToObject(e)
		}
		return NewVectorFrom(objs...)
	case map[string]interface{}:
		m := EmptyArrayMap()
		for k, val := range x {
			m.Add(coretypes.MakeString(k), podPayloadToObject(val))
		}
		return m
	default:
		return coretypes.MakeString(fmt.Sprint(v))
	}
}
