package core

import (
	"time"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func MakeBoolean(b bool) Boolean  { return coretypes.MakeBoolean(b) }
func MakeTime(t time.Time) Time   { return coretypes.MakeTime(t) }
func MakeDouble(d float64) Double { return coretypes.MakeDouble(d) }
func MakeInt(i int) Int           { return coretypes.MakeInt(i) }
func MakeChar(r rune) Char        { return coretypes.MakeChar(r) }
