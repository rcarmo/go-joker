// Package collections is reserved for collection extraction from root core.
//
// It intentionally contains no production collection code yet. Go packages are
// directory-scoped, so vectors, maps, sets, seqs, and transients should move
// here only as real package moves. Current production collection construction
// call sites route through root core's CollectionConstructionAdapter, and the
// layout guard rejects future files in this package that import root core.
// Concrete implementations should move here only after object/protocol
// dependencies are explicit and acyclic.
package collections
