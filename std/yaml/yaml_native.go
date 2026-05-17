package yaml

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"

	"gopkg.in/yaml.v2"

	. "github.com/rcarmo/go-joker/core"
)

func fromObject(obj coretypes.Object) interface{} {
	switch obj := obj.(type) {
	case Keyword:
		return obj.ToString(false)[1:]
	case coretypes.Boolean:
		return obj.B
	case coretypes.Number:
		return obj.Double().D
	case Nil:
		return nil
	case Vec:
		cnt := obj.Count()
		res := make([]interface{}, cnt)
		for i := 0; i < cnt; i++ {
			res[i] = fromObject(obj.Nth(i))
		}
		return res
	case Map:
		res := make(map[string]interface{})
		for iter := obj.Iter(); iter.HasNext(); {
			p := iter.Next()
			var k string
			switch p.Key.(type) {
			case Keyword:
				k = p.Key.ToString(false)[1:]
			default:
				k = p.Key.ToString(false)
			}
			res[k] = fromObject(p.Value)
		}
		return res
	default:
		return obj.ToString(false)
	}
}

func toObject(v interface{}) coretypes.Object {
	switch v := v.(type) {
	case string:
		return coretypes.MakeString(v)
	case float64:
		return coretypes.Double{D: v}
	case int:
		return coretypes.Int{I: v}
	case bool:
		return coretypes.Boolean{B: v}
	case nil:
		return NIL
	case []interface{}:
		res := EmptyVector()
		for _, v := range v {
			res = res.Conjoin(toObject(v))
		}
		return res
	case map[interface{}]interface{}:
		res := EmptyArrayMap()
		for k, v := range v {
			res.Add(toObject(k), toObject(v))
		}
		return res
	default:
		panic(RT.NewError(fmt.Sprintf("Unknown yaml value: %v", v)))
	}
}

func readString(s string) coretypes.Object {
	var v interface{}
	if err := yaml.Unmarshal([]byte(s), &v); err != nil {
		panic(RT.NewError("Invalid yaml: " + err.Error()))
	}
	return toObject(v)
}

func writeString(obj coretypes.Object) coretypes.String {
	res, err := yaml.Marshal(fromObject(obj))
	if err != nil {
		panic(RT.NewError("Cannot encode value to yaml: " + err.Error()))
	}
	return coretypes.String{S: string(res)}
}
