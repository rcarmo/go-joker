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
