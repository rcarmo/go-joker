package core

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var zeroTime time.Time
var irProfileEnabled = os.Getenv("JOKER_IR_PROFILE") != "" || os.Getenv("JOKER_IR_PROFILE_OUT") != ""
var irProfileOut = os.Getenv("JOKER_IR_PROFILE_OUT")
var irProfileCounts [256]atomic.Uint64
var irProfileNanos [256]atomic.Uint64
var irProfileExecs atomic.Uint64
var irProfileMu sync.Mutex
var irProfileEdges = map[[2]byte]uint64{}
var irProfileEdgeNanos = map[[2]byte]uint64{}

func irProfileExecStart() {
	if irProfileEnabled {
		irProfileExecs.Add(1)
	}
}

func irProfileStart() time.Time {
	if !irProfileEnabled {
		return zeroTime
	}
	return time.Now()
}

func irProfileOp(prev byte, op byte, hasPrev bool, prevStarted time.Time) time.Time {
	now := irProfileStart()
	if !irProfileEnabled {
		return now
	}
	irProfileCounts[op].Add(1)
	if hasPrev {
		ns := uint64(now.Sub(prevStarted).Nanoseconds())
		irProfileNanos[prev].Add(ns)
		irProfileMu.Lock()
		key := [2]byte{prev, op}
		irProfileEdges[key]++
		irProfileEdgeNanos[key] += ns
		irProfileMu.Unlock()
	}
	return now
}

func irProfileFinish(last byte, hasLast bool, started time.Time) {
	if !irProfileEnabled || !hasLast {
		return
	}
	irProfileNanos[last].Add(uint64(time.Since(started).Nanoseconds()))
}

func irProfileMaybeWrite() {
	if !irProfileEnabled || irProfileOut == "" {
		return
	}
	irProfileMu.Lock()
	defer irProfileMu.Unlock()
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
	for i := 0; i < len(irProfileCounts); i++ {
		c := irProfileCounts[i].Load()
		if c > 0 {
			ns := irProfileNanos[i].Load()
			avg := uint64(0)
			if c > 0 {
				avg = ns / c
			}
			ops = append(ops, opRow{Op: byte(i), Name: irOpcodeName(byte(i)), Count: c, Nanos: ns, AvgNano: avg})
		}
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].Count > ops[j].Count })
	edges := []edgeRow{}
	for e, c := range irProfileEdges {
		ns := irProfileEdgeNanos[e]
		avg := uint64(0)
		if c > 0 {
			avg = ns / c
		}
		edges = append(edges, edgeRow{Source: irOpcodeName(e[0]), Target: irOpcodeName(e[1]), Count: c, Nanos: ns, AvgNano: avg})
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].Count > edges[j].Count })
	payload := map[string]interface{}{"type": "go-joker-ir-profile", "execs": irProfileExecs.Load(), "ops": ops, "edges": edges}
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(irProfileOut, b, 0o644)
	}
}
