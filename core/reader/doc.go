// Package reader is reserved for reader/parser extraction from root core.
//
// It intentionally contains no production reader code yet. Reader and parser
// files should move here only after object/expression construction boundaries
// are explicit and the package no longer depends on unexported root-core
// internals by reach-through.
package reader
