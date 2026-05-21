package url

import (
	corert "github.com/rcarmo/go-joker/core/runtime"
	"net/url"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"
)

func pathUnescape(s string) string {
	res, err := url.PathUnescape(s)
	if err != nil {
		panic(RT.NewError("Error unescaping string: " + err.Error()))
	}
	return res
}

func queryUnescape(s string) string {
	res, err := url.QueryUnescape(s)
	if err != nil {
		panic(RT.NewError("Error unescaping string: " + err.Error()))
	}
	return res
}

func parseQuery(s string) coretypes.Object {
	values, err := url.ParseQuery(s)
	if err != nil {
		panic(RT.NewError("Error parsing query string: " + err.Error()))
	}
	res := corecollections.EmptyArrayMap()
	for k, v := range values {
		res.Add(coretypes.MakeString(k), corert.MakeStringVector(v))
	}
	return res
}
