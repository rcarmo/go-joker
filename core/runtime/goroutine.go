package runtime

import (
	sdkruntime "runtime"
	"strconv"
	"sync"
)

// GoroutineState tracks per-goroutine state with a fast-path main state.
type GoroutineState struct {
	mainID      int64
	mainState   any
	spawned     int64
	mu          sync.RWMutex
	byGoroutine map[int64]any
}

func NewGoroutineState(mainID int64, mainState any) *GoroutineState {
	return &GoroutineState{mainID: mainID, mainState: mainState, byGoroutine: make(map[int64]any)}
}

func (g *GoroutineState) Current(id int64) any {
	g.mu.RLock()
	if g.spawned == 0 || id == g.mainID {
		v := g.mainState
		g.mu.RUnlock()
		return v
	}
	v, ok := g.byGoroutine[id]
	g.mu.RUnlock()
	if ok {
		return v
	}
	return g.mainState
}

func (g *GoroutineState) Register(id int64, state any) any {
	g.mu.Lock()
	g.byGoroutine[id] = state
	g.spawned++
	g.mu.Unlock()
	return state
}

func (g *GoroutineState) Unregister(id int64) {
	g.mu.Lock()
	if _, ok := g.byGoroutine[id]; ok {
		delete(g.byGoroutine, id)
		g.spawned--
	}
	g.mu.Unlock()
}

// GoRTPool wraps GoroutineState with a goid provider.
type GoRTPool struct {
	state *GoroutineState
	goid  func() int64
}

func NewGoRTPool(goid func() int64, mainState any) *GoRTPool {
	id := goid()
	return &GoRTPool{state: NewGoroutineState(id, mainState), goid: goid}
}

func (p *GoRTPool) Current() any { return p.state.Current(p.goid()) }

func (p *GoRTPool) Register(state any) any { return p.state.Register(p.goid(), state) }

func (p *GoRTPool) Unregister() { p.state.Unregister(p.goid()) }

// GoID extracts the current goroutine ID from the stack header.
// It is intended for cold-path runtime bookkeeping only.
func GoID() int64 {
	var buf [64]byte
	n := sdkruntime.Stack(buf[:], false)
	i := len("goroutine ")
	if n <= i {
		return 0
	}
	j := i
	for j < n && buf[j] >= '0' && buf[j] <= '9' {
		j++
	}
	if j == i {
		return 0
	}
	id, err := strconv.ParseInt(string(buf[i:j]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
