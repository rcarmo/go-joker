package trace

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type IRProfile struct {
	enabled   bool
	out       string
	zero      time.Time
	counts    [256]atomic.Uint64
	nanos     [256]atomic.Uint64
	execs     atomic.Uint64
	mu        sync.Mutex
	edges     map[[2]byte]uint64
	edgeNanos map[[2]byte]uint64
}

func NewIRProfile(enabled bool, out string) *IRProfile {
	return &IRProfile{enabled: enabled, out: out, edges: map[[2]byte]uint64{}, edgeNanos: map[[2]byte]uint64{}}
}

func (p *IRProfile) Enabled() bool { return p != nil && p.enabled }

func (p *IRProfile) ExecStart() {
	if p.Enabled() {
		p.execs.Add(1)
	}
}

func (p *IRProfile) Start() time.Time {
	if !p.Enabled() {
		return p.zero
	}
	return time.Now()
}

func (p *IRProfile) Op(prev byte, op byte, hasPrev bool, prevStarted time.Time) time.Time {
	now := p.Start()
	if !p.Enabled() {
		return now
	}
	p.counts[op].Add(1)
	if hasPrev {
		ns := uint64(now.Sub(prevStarted).Nanoseconds())
		p.nanos[prev].Add(ns)
		p.mu.Lock()
		key := [2]byte{prev, op}
		p.edges[key]++
		p.edgeNanos[key] += ns
		p.mu.Unlock()
	}
	return now
}

func (p *IRProfile) Finish(last byte, hasLast bool, started time.Time) {
	if !p.Enabled() || !hasLast {
		return
	}
	p.nanos[last].Add(uint64(time.Since(started).Nanoseconds()))
}

func (p *IRProfile) Write(opName func(byte) string) {
	if !p.Enabled() || p.out == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	type opRow struct {
		Op      byte   `json:"op"`
		Name    string `json:"name"`
		Count   uint64 `json:"count"`
		Nanos   uint64 `json:"nanos"`
		AvgNano uint64 `json:"avg_nanos"`
	}
	type edgeRow struct {
		Source  string `json:"source"`
		Target  string `json:"target"`
		Count   uint64 `json:"count"`
		Nanos   uint64 `json:"nanos"`
		AvgNano uint64 `json:"avg_nanos"`
	}
	ops := []opRow{}
	for i := 0; i < len(p.counts); i++ {
		if c := p.counts[i].Load(); c > 0 {
			ns := p.nanos[i].Load()
			avg := uint64(0)
			if c > 0 {
				avg = ns / c
			}
			ops = append(ops, opRow{Op: byte(i), Name: opName(byte(i)), Count: c, Nanos: ns, AvgNano: avg})
		}
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].Count > ops[j].Count })
	edges := []edgeRow{}
	for e, c := range p.edges {
		ns := p.edgeNanos[e]
		avg := uint64(0)
		if c > 0 {
			avg = ns / c
		}
		edges = append(edges, edgeRow{Source: opName(e[0]), Target: opName(e[1]), Count: c, Nanos: ns, AvgNano: avg})
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].Count > edges[j].Count })
	payload := map[string]interface{}{"type": "go-joker-ir-profile", "execs": p.execs.Load(), "ops": ops, "edges": edges}
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(p.out, b, 0o644)
	}
}
