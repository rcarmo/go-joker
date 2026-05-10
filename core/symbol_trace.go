package core

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"sync/atomic"
)

var symbolTraceEnabled = os.Getenv("JOKER_SYMBOL_TRACE") != "" || os.Getenv("JOKER_SYMBOL_TRACE_OUT") != ""
var symbolTraceOut = os.Getenv("JOKER_SYMBOL_TRACE_OUT")
var symbolTraceResolveTotal atomic.Uint64
var symbolTraceDerefTotal atomic.Uint64
var symbolTraceMu sync.Mutex
var symbolTraceResolves = map[string]uint64{}
var symbolTraceDerefs = map[string]uint64{}

func traceSymbolResolve(ns *Namespace, sym Symbol, ok bool) {
	if !symbolTraceEnabled || !ok {
		return
	}
	name := sym.ToString(false)
	if ns != nil && sym.ns == nil {
		name = ns.Name.ToString(false) + "/" + name
	}
	symbolTraceResolveTotal.Add(1)
	symbolTraceMu.Lock()
	symbolTraceResolves[name]++
	symbolTraceMu.Unlock()
}

func traceVarDeref(v *Var) {
	if !symbolTraceEnabled || v == nil {
		return
	}
	name := v.name.ToString(false)
	if v.ns != nil {
		name = v.ns.Name.ToString(false) + "/" + v.name.ToString(false)
	}
	symbolTraceDerefTotal.Add(1)
	symbolTraceMu.Lock()
	symbolTraceDerefs[name]++
	symbolTraceMu.Unlock()
}

func symbolTraceMaybeWrite() {
	if !symbolTraceEnabled || symbolTraceOut == "" {
		return
	}
	symbolTraceMu.Lock()
	defer symbolTraceMu.Unlock()
	type row struct {
		Symbol string `json:"symbol"`
		Count  uint64 `json:"count"`
	}
	mkRows := func(m map[string]uint64) []row {
		rows := make([]row, 0, len(m))
		for k, v := range m {
			rows = append(rows, row{Symbol: k, Count: v})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Count > rows[j].Count })
		return rows
	}
	payload := map[string]interface{}{
		"type":          "go-joker-symbol-trace",
		"resolve_total": symbolTraceResolveTotal.Load(),
		"deref_total":   symbolTraceDerefTotal.Load(),
		"resolves":      mkRows(symbolTraceResolves),
		"derefs":        mkRows(symbolTraceDerefs),
	}
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(symbolTraceOut, b, 0o644)
	}
}
