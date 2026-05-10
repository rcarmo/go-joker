package core

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
)

var functionTraceEnabled = os.Getenv("JOKER_FUNCTION_TRACE") != "" || os.Getenv("JOKER_FUNCTION_TRACE_OUT") != ""
var functionTraceOut = os.Getenv("JOKER_FUNCTION_TRACE_OUT")
var functionTraceTotal atomic.Uint64
var functionTraceMu sync.Mutex
var functionTraceCounts = map[string]uint64{}
var functionTraceEdges = map[[2]string]uint64{}
var functionTraceStack []string

func traceFnCall(fn *Fn, argc int) func() {
	if !functionTraceEnabled {
		return func() {}
	}
	name := fnTraceName(fn, argc)
	functionTraceTotal.Add(1)
	functionTraceMu.Lock()
	if len(functionTraceStack) > 0 {
		parent := functionTraceStack[len(functionTraceStack)-1]
		functionTraceEdges[[2]string{parent, name}]++
	}
	functionTraceCounts[name]++
	functionTraceStack = append(functionTraceStack, name)
	functionTraceMu.Unlock()
	return func() {
		functionTraceMu.Lock()
		if len(functionTraceStack) > 0 {
			functionTraceStack = functionTraceStack[:len(functionTraceStack)-1]
		}
		functionTraceMu.Unlock()
		functionTraceMaybeWrite()
	}
}

func traceProcCall(p Proc, argc int) func() {
	if !functionTraceEnabled {
		return func() {}
	}
	name := "proc/" + p.Name
	if p.Package != "" {
		name = p.Package + "/" + p.Name
	}
	name = fmt.Sprintf("%s/%d", name, argc)
	functionTraceTotal.Add(1)
	functionTraceMu.Lock()
	if len(functionTraceStack) > 0 {
		parent := functionTraceStack[len(functionTraceStack)-1]
		functionTraceEdges[[2]string{parent, name}]++
	}
	functionTraceCounts[name]++
	functionTraceStack = append(functionTraceStack, name)
	functionTraceMu.Unlock()
	return func() {
		functionTraceMu.Lock()
		if len(functionTraceStack) > 0 {
			functionTraceStack = functionTraceStack[:len(functionTraceStack)-1]
		}
		functionTraceMu.Unlock()
		functionTraceMaybeWrite()
	}
}

func fnTraceName(fn *Fn, argc int) string {
	if fn.defVar != nil && fn.defVar.ns != nil {
		return fmt.Sprintf("%s/%s/%d", fn.defVar.ns.Name.ToString(false), fn.defVar.name.ToString(false), argc)
	}
	if info := fn.GetInfo(); info != nil {
		return fmt.Sprintf("fn@%s:%d/%d", info.Filename(), info.startLine, argc)
	}
	return fmt.Sprintf("fn@%p/%d", fn, argc)
}

func functionTraceMaybeWrite() {
	if !functionTraceEnabled || functionTraceOut == "" {
		return
	}
	functionTraceMu.Lock()
	defer functionTraceMu.Unlock()
	type row struct {
		Name  string `json:"name"`
		Count uint64 `json:"count"`
	}
	type edge struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Count  uint64 `json:"count"`
	}
	rows := make([]row, 0, len(functionTraceCounts))
	for k, v := range functionTraceCounts {
		rows = append(rows, row{k, v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Count > rows[j].Count })
	edges := make([]edge, 0, len(functionTraceEdges))
	for k, v := range functionTraceEdges {
		edges = append(edges, edge{k[0], k[1], v})
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].Count > edges[j].Count })
	payload := map[string]interface{}{"type": "go-joker-function-trace", "total": functionTraceTotal.Load(), "functions": rows, "edges": edges}
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(functionTraceOut, b, 0o644)
	}
}
