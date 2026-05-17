package url

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"net/url"

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

func parseQuery(s string) Object {
	values, err := url.ParseQuery(s)
	if err != nil {
		panic(RT.NewError("Error parsing query string: " + err.Error()))
	}
	res := EmptyArrayMap()
	for k, v := range values {
		res.Add(coretypes.MakeString(k), MakeStringVector(v))
	}
	return res
}
