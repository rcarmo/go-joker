# Collections extraction audit

Updated: 2026-05-18

Current root collection families are still strongly coupled to root `core` concrete types, metadata, namespace/proc registration, and generated bootstrap behavior. The object/protocol surface has largely moved to `core/types`, so the current safe path is to keep splitting concrete return types and metadata/proc seams before moving implementations.

## 2026-05-18 protocol cleanup update

The shared collection/protocol surface has moved further into `core/types`: `Map`, `MapIterator`, `Pair`, `EmptyMapIterator` and its singleton/error helper, `Meta`, `MetaHolder`, `Set`, `Vec`, `Ref`, and `SafeMerge` are now `coretypes` contracts. Root compatibility aliases for these contracts were removed, and generated assertion output skips `coretypes.*` helpers entirely. Concrete collection implementations still live in root `core`; the remaining work is now a concrete implementation/package move, not a protocol-alias cleanup.


## 2026-05-18 mechanics follow-up update

Collection mechanics moved further into `core/collections`: persistent-vector trie operations, vector trie/storage helpers, hash-map iterator traversal helpers, list-node materialization accessors, and sorted key/value sorting/reverse helpers now live in collections. Root files (`vector.go`, `persistent_vector.go`, `hash_map.go`, `list.go`, `sorted_colls.go`) now delegate those mechanics through explicit `corecollections` helpers while keeping root runtime/proc/type registration behavior.

## Root collection file groups

- Vectors: `array_vector.go`, `vector.go`, `persistent_vector.go`, `persistent_vector_test.go`.
- Maps/sets: `array_map.go`, `hash_map.go`, `map.go`, `set.go`, `sorted_colls.go`, `record.go`, `record_init.go`.
- Seqs/lists: `seq.go`, `list.go`, `chunked_seq.go`, `seq_ops_fast.go`.
- Transients: `transient.go`, `transient_string.go`, `transient_test.go`.
- Reducer/pipeline helpers: `reduce_fast.go`, `reduced.go`, `map_filter_fast.go`.
- Generated/runtime-mutating collection payloads: `a_set_code.go` and collection references inside `a_code.go` / `a_core_code.go`.

## Coupling summary

- Concrete collection types now expose `coretypes.Object`, `coretypes.Seq`, `coretypes.Callable`, `coretypes.CountedIndexed`, `coretypes.Associative`, `coretypes.Collection`, `coretypes.Map`, `coretypes.Set`, `coretypes.Vec`, `coretypes.MetaHolder`, and related moved protocols, but still depend on root concrete implementations such as `ArrayVector`, `VectorSeq`, `ArrayMap`, `HashMap`, `MapSet`, `TYPE`, `RT`, printing, construction adapters, and evaluator-facing behavior.
- Maps/sets depend on object hashing/equality through `coretypes.Object`; their abstract APIs are now `coretypes.Map`/`coretypes.Set`, but implementations still construct and type-assert root `ArrayMap`, `HashMap`, and `MapSet`.
- Vectors depend on root seq views, concrete vector implementations, metadata propagation, printing, and bounds errors through `RT`, even though `Vec`/indexed/stack/counting protocols have moved.
- Seqs/lists are evaluator-adjacent because they carry `coretypes.Callable` reduce paths and concrete root sequence implementations.
- Transients are coupled both to concrete persistent collections and to root proc registration (`GLOBAL_ENV`, `Proc`, `referToUser`).
- `map_filter_fast.go` is not a collection implementation move candidate yet; it is AST/evaluator optimization code and should stay root-bound until evaluator extraction.
- Remaining hash-map expansion/collision code is now mostly coupled to root `Node`, `Object.Hash`, `Object.Equals`, and seq/protocol behavior; pause before forcing larger map moves. Sparse `ArrayNode` compaction delegates to generic collection mechanics while preserving root node ownership.

## Minimal extraction interface candidates

To move concrete implementations without importing root `core`, `core/collections` needs generic or minimal interfaces rather than root concrete collection types:

- Equality/hash hooks: `Equal(a, b T) bool`, `Hash(T) uint32` or a `Hasher[T]`/`Equaler[T]` callback set.
- Bounds/error hooks: return `(value, ok)` or standard errors from mechanics; root adapters translate to `RT.NewError`.
- Metadata/info that only needs source `InfoHolder` can use `coretypes.InfoHolder`; collection metadata contracts now use `coretypes.MetaHolder`/`coretypes.Map`, but concrete metadata construction/propagation and `TYPE` registration stay root-side initially.
- Printing/formatting stays root-side initially.
- Callable/proc registration stays root-side.

## Mechanics extracted so far

`core/collections` now owns root-independent mechanics with direct package tests:

- `vector_storage.go`: generic clone/append/assoc/assoc2/pop/from-values slice helpers.
- `trie_node.go`: opaque fixed-width trie node clone/path helpers used by persistent-vector trie mechanics.
- `pair_storage.go`: generic pair-array append/insert/remove helpers and sparse indexed-node compaction used by hash-map node arrays.
- `bitmap.go`: bit-count/hash-mask/bitmap-position helpers used by hash-map indexing.
- `list_node.go`: generic persistent cons-node storage used by root `List` while root retains Object/protocol/generated-bootstrap adapter fields.
- `map_equality.go`: generic map equality loop used by root maps while root supplies Object equality, lookup, and iteration adapters.
- `format.go`: generic delimited and pair-delimited string rendering used by root map/set string forms while root supplies Object string conversion.

Root `ArrayVector`, legacy `Vector`, `PersistentVector`, `List`, maps, and sets delegate to these helpers where safe. Most abstract Object/protocol behavior now lives in `core/types`; concrete collection behavior remains in root.

## Safe first move candidate

The safest first real package move is a root-independent mechanics helper, not a public root collection type:

1. Extract a small vector/list storage primitive into `core/collections` using ordinary Go values and no root imports.
2. Add package-local tests for clone/append/assoc/pop or chunk indexing semantics.
3. Make root `ArrayVector`/future adapters delegate only storage operations to that primitive while retaining `Object`, metadata, seq, printing, `TYPE`, `RT`, and callable behavior in root.

A good first concrete candidate is an `Object`-agnostic generic slice helper for vector storage operations, e.g. clone/append/assoc/pop with tests. **Started:** `core/collections` now owns generic slice storage helpers, generic persistent list-node storage, generic map equality traversal, generic delimited formatting, pair-array helpers, bitmap/hash-index helpers, sparse indexed-node packing, and opaque trie node/path helpers; root `ArrayVector`, legacy `Vector`, `PersistentVector` tail/trie operations, root `List` storage, root map equality/string rendering loops, root set string rendering loops, and hash-map node/pair-array/bitmap/packing mechanics delegate clone/append/assoc/pop/from-values/list-node/map-equality/formatting/node-copy/path/pair insert/pair remove/bit count/hash mask/single-slot and double-slot assoc mechanics to them while retaining concrete collection/metadata behavior in root. Once that seam is stable, the same pattern can be applied to more map/set bucket mechanics.

## Explicit non-candidates for immediate move

- `seq_ops_fast.go` and `map_filter_fast.go`: evaluator/Callable-heavy.
- `transient.go`: depends on persistent vector/map internals and root proc registration.
- `sorted_colls.go`: runtime registration and comparator/callable behavior.
- Generated `a_*_code.go`: runtime-mutating bootstrap until equivalence and generator seams are proven.
