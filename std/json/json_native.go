package json

import (
	"encoding/json"
	"fmt"
	corert "github.com/rcarmo/go-joker/core/runtime"
	"io"
	"strings"

	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

func fromObject(obj coretypes.Object) interface{} {
	switch obj := obj.(type) {
	case coretypes.Keyword:
		return obj.ToString(false)[1:]
	case coretypes.Boolean:
		return obj.B
	case coretypes.Number:
		return obj.Double().D
	case Nil:
		return nil
	case coretypes.String:
		return obj.ToString(false)
	case coretypes.Map:
		res := make(map[string]interface{})
		for iter := obj.Iter(); iter.HasNext(); {
			p := iter.Next()
			var k string
			switch p.Key.(type) {
			case coretypes.Keyword:
				k = p.Key.ToString(false)[1:]
			default:
				k = p.Key.ToString(false)
			}
			res[k] = fromObject(p.Value)
		}
		return res
	case coretypes.Seqable:
		s := obj.Seq()
		var res []interface{} = []interface{}{}
		for !s.IsEmpty() {
			res = append(res, fromObject(s.First()))
			s = s.Rest()
		}
		return res
	default:
		return obj.ToString(false)
	}
}

func toObject(v interface{}, keywordize bool) coretypes.Object {
	switch v := v.(type) {
	case string:
		return coretypes.MakeString(v)
	case float64:
		if v == float64(int(v)) {
			return coretypes.Int{I: int(v)}
		}
		return coretypes.Double{D: v}
	case bool:
		return coretypes.Boolean{B: v}
	case nil:
		return NIL
	case []interface{}:
		res := corecollections.EmptyVector()
		for _, v := range v {
			res = res.Conjoin(toObject(v, keywordize))
		}
		return res
	case map[string]interface{}:
		res := corecollections.EmptyArrayMap()
		for k, v := range v {
			var key coretypes.Object
			if keywordize {
				key = coretypes.MakeKeyword(STRINGS.Intern, k)
			} else {
				key = coretypes.MakeString(k)
			}
			res.Add(key, toObject(v, keywordize))
		}
		return res
	default:
		panic(RT.NewError(fmt.Sprintf("Unknown json value: %v", v)))
	}
}

func readString(s string, opts coretypes.Map) coretypes.Object {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(RT.NewError("Invalid json: " + err.Error()))
	}
	var keywordize bool
	if opts != nil {
		if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "keywords?")); ok {
			keywordize = corert.ToBool(v)
		}
	}
	return toObject(v, keywordize)
}

func jsonSeqOpts(src coretypes.Object, opts coretypes.Map) coretypes.Object {
	var dec *json.Decoder
	var keywordize bool
	var jsonLazySeq func() *corecollections.LazySeq
	switch src := src.(type) {
	case coretypes.String:
		dec = json.NewDecoder(strings.NewReader(src.S))
	case io.Reader:
		dec = json.NewDecoder(src)
	default:
		panic(RT.NewError("src must be a string or io.Reader"))
	}
	if opts != nil {
		if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "keywords?")); ok {
			keywordize = corert.ToBool(v)
		}
	}
	jsonLazySeq = func() *corecollections.LazySeq {
		var c = func(args []coretypes.Object) coretypes.Object {
			var o interface{}
			err := dec.Decode(&o)
			if err == io.EOF {
				return corecollections.EmptyList
			}
			if err != nil {
				panic(RT.NewError("Cannot decode json stream: " + err.Error()))
			}
			obj := toObject(o, keywordize)
			return corecollections.NewConsSeq(obj, jsonLazySeq())
		}
		return corecollections.NewLazySeq(Proc{Fn: c})
	}
	return jsonLazySeq()
}

func writeString(obj coretypes.Object, opts coretypes.Map) coretypes.String {
	var (
		prefix coretypes.String
		indent coretypes.String
		res    []byte
		err    error
	)
	if opts != nil {
		if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "prefix")); ok {
			prefix = coretypes.EnsureObjectIsString(v, "prefix: %s")
		}
		if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "indent")); ok {
			indent = coretypes.EnsureObjectIsString(v, "indent: %s")
		}
	}

	if prefix.S != "" || indent.S != "" {
		res, err = json.MarshalIndent(fromObject(obj), prefix.S, indent.S)
	} else {
		res, err = json.Marshal(fromObject(obj))
	}
	if err != nil {
		panic(RT.NewError("Cannot encode value to json: " + err.Error()))
	}
	return coretypes.String{S: string(res)}
}
