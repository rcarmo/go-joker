// Package runtime owns extracted leaf runtime helpers.
//
// Root evaluator/executor code still lives in package core, but low-coupling
// runtime bookkeeping helpers can move here once they no longer depend on root
// object, call, error, or frame internals. Current executor state is narrowed
// behind root core's RuntimeExecutionAdapter; the layout guard rejects future
// files in this package that import root core, so boxed/typed executor moves
// must first make object/call/error/frame dependencies explicit and acyclic.
package runtime
