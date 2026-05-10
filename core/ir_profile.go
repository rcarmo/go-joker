package core

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"sync/atomic"
)

var irProfileEnabled = os.Getenv("JOKER_IR_PROFILE") != "" || os.Getenv("JOKER_IR_PROFILE_OUT") != ""
var irProfileOut = os.Getenv("JOKER_IR_PROFILE_OUT")
var irProfileCounts [256]atomic.Uint64
var irProfileExecs atomic.Uint64
var irProfileMu sync.Mutex
var irProfileEdges = map[[2]byte]uint64{}

func irProfileExecStart() {
	if irProfileEnabled {
		irProfileExecs.Add(1)
	}
}

func irProfileOp(prev byte, op byte, hasPrev bool) {
	if !irProfileEnabled {
		return
	}
	irProfileCounts[op].Add(1)
	if hasPrev {
		irProfileMu.Lock()
		irProfileEdges[[2]byte{prev, op}]++
		irProfileMu.Unlock()
	}
}

func irProfileMaybeWrite() {
	if !irProfileEnabled || irProfileOut == "" {
		return
	}
	irProfileMu.Lock()
	defer irProfileMu.Unlock()
	type opRow struct {
		Op    byte   `json:"op"`
		Name  string `json:"name"`
		Count uint64 `json:"count"`
	}
	type edgeRow struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Count  uint64 `json:"count"`
	}
	ops := []opRow{}
	for i := 0; i < len(irProfileCounts); i++ {
		c := irProfileCounts[i].Load()
		if c > 0 {
			ops = append(ops, opRow{Op: byte(i), Name: irOpcodeName(byte(i)), Count: c})
		}
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].Count > ops[j].Count })
	edges := []edgeRow{}
	for e, c := range irProfileEdges {
		edges = append(edges, edgeRow{Source: irOpcodeName(e[0]), Target: irOpcodeName(e[1]), Count: c})
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].Count > edges[j].Count })
	payload := map[string]interface{}{"type": "go-joker-ir-profile", "execs": irProfileExecs.Load(), "ops": ops, "edges": edges}
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(irProfileOut, b, 0o644)
	}
}
