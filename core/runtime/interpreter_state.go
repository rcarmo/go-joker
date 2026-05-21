package runtime

// GoroutineRT holds per-goroutine interpreter state without importing root core.
// CurrentExpr is intentionally `any`; root narrows it to its Expr/Traceable types.
type GoroutineRT struct {
	Callstack   *Callstack
	CurrentExpr any
}

func NewGoroutineRT(stackCapacity int) *GoroutineRT {
	return &GoroutineRT{Callstack: NewCallstack(stackCapacity)}
}

func (g *GoroutineRT) Clone() *GoroutineRT {
	return &GoroutineRT{Callstack: g.Callstack.Clone(), CurrentExpr: g.CurrentExpr}
}

// InterpreterStatePool owns per-goroutine interpreter state with a typed API.
type InterpreterStatePool struct {
	pool *GoRTPool
}

func NewInterpreterStatePool(main *GoroutineRT) *InterpreterStatePool {
	return &InterpreterStatePool{pool: NewGoRTPool(GoID, main)}
}

func (p *InterpreterStatePool) Current() *GoroutineRT {
	return p.pool.Current().(*GoroutineRT)
}

func (p *InterpreterStatePool) Register(stackCapacity int) *GoroutineRT {
	state := NewGoroutineRT(stackCapacity)
	p.pool.Register(state)
	return state
}

func (p *InterpreterStatePool) Unregister() {
	p.pool.Unregister()
}
