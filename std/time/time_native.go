package time

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math/big"
	"time"

	. "github.com/rcarmo/go-joker/core"
)

func timeIntObject(n int64) Object {
	maxNativeInt := int64(int(^uint(0) >> 1))
	minNativeInt := -maxNativeInt - 1
	if n > maxNativeInt || n < minNativeInt {
		return coretypes.MakeBigInt(big.NewInt(n))
	}
	return coretypes.MakeInt(int(n))
}

func inTimezone(t time.Time, tz string) time.Time {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		panic(RT.NewError("time/in-timezone: " + err.Error()))
	}
	return t.In(loc)
}

func parseInTimezone(layout string, value string, tz string) time.Time {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		panic(RT.NewError("time/parse-in-timezone: " + err.Error()))
	}
	t, err := time.ParseInLocation(layout, value, loc)
	if err != nil {
		panic(RT.NewError("time/parse-in-timezone: " + err.Error()))
	}
	return t
}
