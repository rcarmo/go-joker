package trace

import (
	"encoding/json"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type Frame struct {
	name       string
	childNanos uint64
}

type Event struct {
	Name           string `json:"name"`
	Parent         string `json:"parent,omitempty"`
	Depth          int    `json:"depth"`
	Start          uint64 `json:"start_nanos"`
	Nanos          uint64 `json:"nanos"`
	ExclusiveNanos uint64 `json:"exclusive_nanos"`
}

type FunctionTracer struct {
	enabled bool
	out     string
	t0      time.Time
	total   atomic.Uint64
	mu      sync.Mutex
	counts  map[string]uint64
	nanos   map[string]uint64
	edges   map[[2]string]uint64
	edgeNs  map[[2]string]uint64
	events  []Event
	stacks  map[uint64][]Frame
}

func NewFunctionTracer(enabled bool, out string) *FunctionTracer {
	return &FunctionTracer{
		enabled: enabled,
		out:     out,
		t0:      time.Now(),
		counts:  map[string]uint64{},
		nanos:   map[string]uint64{},
		edges:   map[[2]string]uint64{},
		edgeNs:  map[[2]string]uint64{},
		stacks:  map[uint64][]Frame{},
	}
}

func (t *FunctionTracer) Enabled() bool { return t != nil && t.enabled }

func (t *FunctionTracer) Enter(name string) func() {
	if !t.Enabled() {
		return func() {}
	}
	start := time.Now()
	startRel := uint64(start.Sub(t.t0).Nanoseconds())
	gid := goroutineID()
	t.total.Add(1)
	t.mu.Lock()
	stack := t.stacks[gid]
	depth := len(stack)
	parent := ""
	if depth > 0 {
		parent = stack[depth-1].name
		t.edges[[2]string{parent, name}]++
	}
	t.counts[name]++
	t.stacks[gid] = append(stack, Frame{name: name})
	t.mu.Unlock()
	return func() {
		t.exit(gid, name, parent, depth, startRel, uint64(time.Since(start).Nanoseconds()))
	}
}

func (t *FunctionTracer) exit(gid uint64, name, parent string, depth int, startRel, dur uint64) {
	t.mu.Lock()
	stack := t.stacks[gid]
	idx := len(stack) - 1
	for idx >= 0 && stack[idx].name != name {
		idx--
	}
	if idx >= 0 {
		frame := stack[idx]
		stack = append(stack[:idx], stack[idx+1:]...)
		exclusive := dur
		if frame.childNanos < dur {
			exclusive = dur - frame.childNanos
		} else {
			exclusive = 0
		}
		t.nanos[name] += dur
		if len(t.events) < 200000 {
			t.events = append(t.events, Event{Name: name, Parent: parent, Depth: depth, Start: startRel, Nanos: dur, ExclusiveNanos: exclusive})
		}
		if idx > 0 {
			parent := stack[idx-1].name
			t.edgeNs[[2]string{parent, name}] += dur
			stack[idx-1].childNanos += dur
		}
		if len(stack) == 0 {
			delete(t.stacks, gid)
		} else {
			t.stacks[gid] = stack
		}
	}
	shouldWrite := len(t.stacks) == 0
	t.mu.Unlock()
	if shouldWrite {
		t.Write()
	}
}

func (t *FunctionTracer) Write() {
	if !t.Enabled() || t.out == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
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
	rows := make([]row, 0, len(t.counts))
	for k, v := range t.counts {
		ns := t.nanos[k]
		avg := uint64(0)
		if v > 0 {
			avg = ns / v
		}
		rows = append(rows, row{Name: k, Count: v, Nanos: ns, AvgNano: avg})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Nanos > rows[j].Nanos })
	edges := make([]edge, 0, len(t.edges))
	for k, v := range t.edges {
		ns := t.edgeNs[k]
		avg := uint64(0)
		if v > 0 {
			avg = ns / v
		}
		edges = append(edges, edge{Source: k[0], Target: k[1], Count: v, Nanos: ns, AvgNano: avg})
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].Nanos > edges[j].Nanos })
	payload := map[string]interface{}{"type": "go-joker-function-trace", "total": t.total.Load(), "functions": rows, "edges": edges, "events": t.events}
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(t.out, b, 0o644)
	}
}

func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	start := len("goroutine ")
	end := start
	for end < n && buf[end] >= '0' && buf[end] <= '9' {
		end++
	}
	id, _ := strconv.ParseUint(string(buf[start:end]), 10, 64)
	return id
}
