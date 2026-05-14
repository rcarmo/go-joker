package core

import (
	"time"

	coreir "github.com/rcarmo/go-joker/core/ir"
	coretrace "github.com/rcarmo/go-joker/core/trace"
)

var symbolTracer = coretrace.NewSymbolTracerFromEnv()
var zeroTime time.Time
var irProfile = coretrace.NewIRProfileFromEnv()

func traceSymbolResolve(ns *Namespace, sym Symbol, ok bool) {
	if !symbolTracer.Enabled() || !ok {
		return
	}
	name := sym.ToString(false)
	if ns != nil && sym.ns == nil {
		name = ns.Name.ToString(false) + "/" + name
	}
	symbolTracer.Resolve(name)
}

func traceVarDeref(v *Var) {
	if !symbolTracer.Enabled() || v == nil {
		return
	}
	name := v.name.ToString(false)
	if v.ns != nil {
		name = v.ns.Name.ToString(false) + "/" + v.name.ToString(false)
	}
	symbolTracer.Deref(name)
}

func symbolTraceMaybeWrite() {
	symbolTracer.Write()
}

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
