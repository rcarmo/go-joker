// Package collections owns reusable collection mechanics and is the target
// package for concrete collection implementations as root core is split up.
//
// It contains consolidated slice storage, indexed operations, persistent
// list/trie mechanics, map/set/seq helpers, chunked sequence types, sorting,
// formatting, HAMT support, and transient collection helpers. Concrete root
// collection files should move here as whole ownership batches, with call sites
// updated mechanically and no root import cycles introduced.
package collections
