// Package reader is reserved for reader/parser extraction from root core.
//
// It intentionally contains no production reader code yet. Reader and parser
// files should move here only as real package moves. Current production reader
// and expression construction call sites route through root core's
// ReaderConstructionAdapter, and the layout guard rejects future files in this
// package that import root core. Concrete reader/parser implementation should
// move here only after object construction, tagged-literal, namespace, error,
// and evaluator handoff dependencies are explicit and acyclic.
package reader
