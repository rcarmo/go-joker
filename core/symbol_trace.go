package core

import (
	"os"

	coretrace "github.com/rcarmo/go-joker/core/internal/trace"
)

var symbolTraceEnabled = os.Getenv("JOKER_SYMBOL_TRACE") != "" || os.Getenv("JOKER_SYMBOL_TRACE_OUT") != ""
var symbolTracer = coretrace.NewSymbolTracer(symbolTraceEnabled, os.Getenv("JOKER_SYMBOL_TRACE_OUT"))

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
