package trace

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"sync/atomic"
)

type SymbolTracer struct {
	enabled  bool
	out      string
	resolve  atomic.Uint64
	deref    atomic.Uint64
	mu       sync.Mutex
	resolves map[string]uint64
	derefs   map[string]uint64
}

func NewSymbolTracer(enabled bool, out string) *SymbolTracer {
	return &SymbolTracer{enabled: enabled, out: out, resolves: map[string]uint64{}, derefs: map[string]uint64{}}
}

func (t *SymbolTracer) Enabled() bool { return t != nil && t.enabled }

func (t *SymbolTracer) Resolve(name string) {
	if !t.Enabled() || name == "" {
		return
	}
	t.resolve.Add(1)
	t.mu.Lock()
	t.resolves[name]++
	t.mu.Unlock()
}

func (t *SymbolTracer) Deref(name string) {
	if !t.Enabled() || name == "" {
		return
	}
	t.deref.Add(1)
	t.mu.Lock()
	t.derefs[name]++
	t.mu.Unlock()
}

func (t *SymbolTracer) Write() {
	if !t.Enabled() || t.out == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
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
	payload := map[string]interface{}{"type": "go-joker-symbol-trace", "resolve_total": t.resolve.Load(), "deref_total": t.deref.Load(), "resolves": mkRows(t.resolves), "derefs": mkRows(t.derefs)}
	if b, err := json.MarshalIndent(payload, "", "  "); err == nil {
		_ = os.WriteFile(t.out, b, 0o644)
	}
}
