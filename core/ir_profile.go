package core

import (
	"os"
	"time"

	coreir "github.com/rcarmo/go-joker/core/ir"
	coretrace "github.com/rcarmo/go-joker/core/trace"
)

var zeroTime time.Time
var irProfile = coretrace.NewIRProfile(os.Getenv("JOKER_IR_PROFILE") != "" || os.Getenv("JOKER_IR_PROFILE_OUT") != "", os.Getenv("JOKER_IR_PROFILE_OUT"))

func irProfileExecStart() {
	irProfile.ExecStart()
}

func irProfileStart() time.Time {
	if !irProfile.Enabled() {
		return zeroTime
	}
	return irProfile.Start()
}

func irProfileOp(prev byte, op byte, hasPrev bool, prevStarted time.Time) time.Time {
	return irProfile.Op(prev, op, hasPrev, prevStarted)
}

func irProfileFinish(last byte, hasLast bool, started time.Time) {
	irProfile.Finish(last, hasLast, started)
}

func irProfileMaybeWrite() {
	irProfile.Write(coreir.OpcodeName)
}
