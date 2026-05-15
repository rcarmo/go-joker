# Collections extraction audit

Current root collection families are still strongly coupled to root `core` contracts. The next moves should split storage/mechanics from Object/protocol adapters before moving concrete implementations.

## Root collection file groups

- Vectors: `array_vector.go`, `vector.go`, `persistent_vector.go`, `persistent_vector_test.go`.
- Maps/sets: `array_map.go`, `hash_map.go`, `map.go`, `set.go`, `sorted_colls.go`, `record.go`, `record_init.go`.
- Seqs/lists: `seq.go`, `list.go`, `chunked_seq.go`, `seq_ops_fast.go`.
- Transients: `transient.go`, `transient_string.go`, `transient_test.go`.
- Reducer/pipeline helpers: `reduce_fast.go`, `reduced.go`, `map_filter_fast.go`.
- Generated/runtime-mutating collection payloads: `a_set_code.go` and collection references inside `a_code.go` / `a_core_code.go`.

## Coupling summary

- All concrete collection types currently expose root `Object`, `Seq`, `Map`, `Set`, `Callable`, `InfoHolder`, `MetaHolder`, `TYPE`, `RT`, printing, hashing, equality, and/or evaluator-facing behavior.
- Maps/sets depend on root object hashing/equality and persistent collection protocols.
- Vectors depend on root `Object`, seq views, `Vec`/`CountedIndexed`, metadata, printing, and bounds errors through `RT`.
- Seqs/lists are evaluator-adjacent because they carry `Callable` reduce paths and root seq protocol behavior.
- Transients are coupled both to concrete persistent collections and to root proc registration (`GLOBAL_ENV`, `Proc`, `referToUser`).
- `map_filter_fast.go` is not a collection implementation move candidate yet; it is AST/evaluator optimization code and should stay root-bound until evaluator extraction.

## Minimal extraction interface candidates

To move storage/mechanics without importing root `core`, `core/collections` needs generic or minimal interfaces rather than root `Object`:

- Equality/hash hooks: `Equal(a, b T) bool`, `Hash(T) uint32` or a `Hasher[T]`/`Equaler[T]` callback set.
- Bounds/error hooks: return `(value, ok)` or standard errors from mechanics; root adapters translate to `RT.NewError`.
- Metadata/info stays root-side initially; moved mechanics should not own `InfoHolder`, `MetaHolder`, or `TYPE`.
- Printing/formatting stays root-side initially.
- Callable/proc registration stays root-side.

## Safe first move candidate

The safest first real package move is a root-independent mechanics helper, not a public root collection type:

1. Extract a small vector/list storage primitive into `core/collections` using ordinary Go values and no root imports.
2. Add package-local tests for clone/append/assoc/pop or chunk indexing semantics.
3. Make root `ArrayVector`/future adapters delegate only storage operations to that primitive while retaining `Object`, metadata, seq, printing, `TYPE`, `RT`, and callable behavior in root.

A good first concrete candidate is an `Object`-agnostic generic slice helper for vector storage operations, e.g. clone/append/assoc/pop with tests. **Started:** `core/collections` now owns generic slice storage helpers, pair-array helpers, bitmap/hash-index helpers, and opaque trie node/path helpers; root `ArrayVector`, legacy `Vector`, `PersistentVector` tail/trie operations, and hash-map node/pair-array/bitmap mechanics delegate clone/append/assoc/pop/from-values/node-copy/path/pair insert/pair remove/bit count/hash mask mechanics to them while retaining Object/protocol behavior in root. Once that seam is stable, the same pattern can be applied to more map/set bucket mechanics.

## Explicit non-candidates for immediate move

- `seq_ops_fast.go` and `map_filter_fast.go`: evaluator/Callable-heavy.
- `transient.go`: depends on persistent vector/map internals and root proc registration.
- `sorted_colls.go`: runtime registration and comparator/callable behavior.
- Generated `a_*_code.go`: runtime-mutating bootstrap until equivalence and generator seams are proven.
