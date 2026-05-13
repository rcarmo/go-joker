// Package runtime owns extracted leaf runtime helpers.
//
// Root evaluator/executor code still lives in package core, but low-coupling
// runtime bookkeeping helpers can move here once they no longer depend on root
// object, call, error, or frame internals.
package runtime
