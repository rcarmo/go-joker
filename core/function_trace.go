package core

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var functionTraceEnabled = os.Getenv("JOKER_FUNCTION_TRACE") != "" || os.Getenv("JOKER_FUNCTION_TRACE_OUT") != ""
var functionTraceOut = os.Getenv("JOKER_FUNCTION_TRACE_OUT")
var functionTraceTotal atomic.Uint64
var functionTraceT0 = time.Now()
var functionTraceMu sync.Mutex
var functionTraceCounts = map[string]uint64{}
var functionTraceNanos = map[string]uint64{}
var functionTraceEdges = map[[2]string]uint64{}
var functionTraceEdgeNanos = map[[2]string]uint64{}

type functionTraceFrame struct {
	name       string
	childNanos uint64
}
type functionTraceEvent struct {
	Name           string `json:"name"`
	Parent         string `json:"parent,omitempty"`
	Depth          int    `json:"depth"`
	Start          uint64 `json:"start_nanos"`
	Nanos          uint64 `json:"nanos"`
	ExclusiveNanos uint64 `json:"exclusive_nanos"`
}

var functionTraceEvents []functionTraceEvent

var functionTraceStack []functionTraceFrame

func traceFnCall(fn *Fn, argc int) func() {
	if !functionTraceEnabled {
		return func() {}
	}
	return traceFunctionEnter(fnTraceName(fn, argc))
}

func traceIRProgramCall(prog *IRProgram, argc int) func() {
	if !functionTraceEnabled || prog == nil || prog.traceName == "" {
		return func() {}
	}
	return traceFunctionEnter(fmt.Sprintf("%s/%d", prog.traceName, argc))
}

func traceProcCall(p Proc, argc int) func() {
	if !functionTraceEnabled {
		return func() {}
	}
	name := "proc/" + p.Name
	if p.Package != "" {
		name = p.Package + "/" + p.Name
	}
	return traceFunctionEnter(fmt.Sprintf("%s/%d", name, argc))
}

func traceFunctionEnter(name string) func() {
	start := time.Now()
	startRel := uint64(start.Sub(functionTraceT0).Nanoseconds())
	functionTraceTotal.Add(1)
	functionTraceMu.Lock()
	depth := len(functionTraceStack)
	parent := ""
	if depth > 0 {
		parent = functionTraceStack[depth-1].name
		functionTraceEdges[[2]string{parent, name}]++
	}
	functionTraceCounts[name]++
	functionTraceStack = append(functionTraceStack, functionTraceFrame{name: name})
	functionTraceMu.Unlock()
	return func() {
		dur := uint64(time.Since(start).Nanoseconds())
		functionTraceMu.Lock()
		idx := len(functionTraceStack) - 1
		if idx >= 0 {
			frame := functionTraceStack[idx]
			functionTraceStack = functionTraceStack[:idx]
			exclusive := dur
			if frame.childNanos < dur {
				exclusive = dur - frame.childNanos
			} else {
				exclusive = 0
			}
			functionTraceNanos[name] += dur
			if len(functionTraceEvents) < 200000 {
				functionTraceEvents = append(functionTraceEvents, functionTraceEvent{Name: name, Parent: parent, Depth: depth, Start: startRel, Nanos: dur, ExclusiveNanos: exclusive})
			}
			if idx > 0 {
				parent := functionTraceStack[idx-1].name
				functionTraceEdgeNanos[[2]string{parent, name}] += dur
				functionTraceStack[idx-1].childNanos += dur
			}
		}
		functionTraceMu.Unlock()
		functionTraceMaybeWrite()
	}
}

func fnTraceName(fn *Fn, argc int) string {
	if fn.defVar != nil {
		if fn.defVar.ns != nil {
			return fmt.Sprintf("%s/%s/%d", fn.defVar.ns.Name.ToString(false), fn.defVar.name.ToString(false), argc)
		}
		return fmt.Sprintf("%s/%d", fn.defVar.name.ToString(false), argc)
	}
	if fn.fnExpr != nil && fn.fnExpr.traceName != "" {
		return fmt.Sprintf("%s/%d", fn.fnExpr.traceName, argc)
	}
	if fn.fnExpr != nil && fn.fnExpr.self.name != nil {
		return fmt.Sprintf("%s/%d", fn.fnExpr.self.ToString(false), argc)
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
		Name    string `json:"name"`
		Count   uint64 `json:"count"`
		Nanos   uint64 `json:"nanos"`
		AvgNano uint64 `json:"avg_nanos"`
	}
	type edge struct {
		Source  string `json:"source"`
		Target  string `json:"target"`
		Count   uint64 `json:"count"`
		Nanos   uint64 `json:"nanos"`
		AvgNano uint64 `json:"avg_nanos"`
	}
	rows := make([]row, 0, len(functionTraceCounts))
	for k, v := range functionTraceCounts {
		ns := functionTraceNanos[k]
		avg := uint64(0)
		if v > 0 {
			avg = ns / v
		}
		rows = append(rows, row{Name: k, Count: v, Nanos: ns, AvgNano: avg})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Nanos > rows[j].Nanos })
	edges := make([]edge, 0, len(functionTraceEdges))
	for k, v := range functionTraceEdges {
		ns := functionTraceEdgeNanos[k]
		avg := uint64(0)
		if v > 0 {
			avg = ns / v
		}
		edges = append(edges, edge{Source: k[0], Target: k[1], Count: v, Nanos: ns, AvgNano: avg})
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].Nanos > edges[j].Nanos })
	payload := map[string]interface{}{"type": "go-joker-function-trace", "total": functionTraceTotal.Load(), "functions": rows, "edges": edges, "events": functionTraceEvents}
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(functionTraceOut, b, 0o644)
	}
}
