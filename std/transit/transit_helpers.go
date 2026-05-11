package transit

import (
	"encoding/json"

	. "github.com/rcarmo/go-joker/core"
)

// TransitEncodeArgs encodes pod invocation arguments as a Transit+JSON list.
func TransitEncodeArgs(args []Object) (string, error) {
	enc := &transitEncoder{cache: newTransitCache()}
	items := make([]interface{}, len(args))
	for i, arg := range args {
		items[i] = enc.encode(arg, false)
	}
	bs, err := json.Marshal([]interface{}{enc.cacheString("~#list", false), items})
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

// TransitDecodeValue decodes a Transit+JSON pod result payload.
func TransitDecodeValue(s string) (result Object, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = NIL
			err = RT.NewError("transit decode failed")
		}
	}()
	return readTransit(MakeString(s)), nil
}
