// Package collections is reserved for collection extraction from root core.
//
// It intentionally contains no production collection code yet. Go packages are
// directory-scoped, so vectors, maps, sets, seqs, and transients should move
// here only after construction/adaptation contracts are explicit and callers no
// longer depend on unexported root-core internals.
package collections
