package time

import (
	"math/big"
	"time"

	. "github.com/rcarmo/go-joker/core"
)

func timeIntObject(n int64) Object {
	maxNativeInt := int64(int(^uint(0) >> 1))
	minNativeInt := -maxNativeInt - 1
	if n > maxNativeInt || n < minNativeInt {
		return MakeBigInt(big.NewInt(n))
	}
	return MakeInt(int(n))
}

func inTimezone(t time.Time, tz string) time.Time {
	loc, err := time.LoadLocation(tz)
	PanicOnErr(err)
	return t.In(loc)
}

func parseInTimezone(layout string, value string, tz string) time.Time {
	loc, err := time.LoadLocation(tz)
	PanicOnErr(err)
	t, err := time.ParseInLocation(layout, value, loc)
	PanicOnErr(err)
	return t
}
