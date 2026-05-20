# Collections extraction audit

Updated: 2026-05-20

Concrete collection ownership has moved out of root `core` and into `core/types/collections` for the main collection families. Root `core` now keeps proc/env registration, evaluator optimizations, sorted collection registration, transients, records, and other runtime glue that still depends on namespaces, `Proc`, `GLOBAL_ENV`, expression/evaluator state, or generated bootstrap wiring.

## 2026-05-20 concrete collection move update

`core/types/collections` now owns:

- vectors: `ArrayVector`, legacy trie `Vector`, `PersistentVector`, `VectorSeq`, `VectorRSeq`, and vector-owned seq helpers;
- seq/list types: `ConsSeq`, `ArraySeq`, `LazySeq`, `MappingSeq`, `List`, and `EmptyList`;
- maps/sets: `ArrayMap`, `ArrayMapSeq`, `HashMap`, HAMT nodes/iterators/seqs, `MapSet`, `EmptyHashMap`, and `HASHMAP_THRESHOLD`;
- chunks: `ChunkBuffer`, `ArrayChunk`, `ChunkedCons`, and chunk mechanics.

The old root files are deleted and guarded against reintroduction by `tests/layout_guard.sh`:

- `core/array_map.go`
- `core/array_vector.go`
- `core/chunked_seq.go`
- `core/hash_map.go`
- `core/list.go`
- `core/map.go`
- `core/persistent_vector.go`
- `core/seq.go`
- `core/set.go`
- `core/vector.go`

Chunk proc registration remains root-owned intentionally but has been coalesced into `core/procs.go`: it registers chunk procs into `GLOBAL_ENV` and uses `Proc`, `STRINGS`, and `referToUser`; arity/error behavior goes through `core/types` runtime hooks. It is proc/env registration, not collection data ownership.

## Current root collection-adjacent file groups

- Sorted collections: sorted collection proc registration and comparator/callable behavior are coalesced into `procs.go`; arity/error behavior goes through `core/types` runtime hooks rather than direct root helpers.
- Records, protocols, hierarchy, and namespaces: record and hierarchy behavior is coalesced into `protocol.go`, while `ns.go` and related files use moved collection types but remain root-owned runtime/object glue; record object arity/error behavior goes through `core/types` runtime hooks rather than direct root helpers.
- Transients: transient proc behavior and conversion hooks are coalesced into `procs.go`, while persistent map/vector implementations are in `core/types/collections`; arity/error behavior goes through `core/types` runtime hooks rather than direct root helpers.
- Reducer/pipeline helpers: `reduce_fast.go` owns reduce, transducer compatibility, `Reduced`, and map/filter/take/range/transducer fast paths as evaluator/runtime optimizations, not collection storage ownership; arity/error behavior in the range/frequencies/transducer glue goes through `core/types` runtime hooks rather than direct root helpers.
- Generated/bootstrap payloads: generated files now reference moved collection types through `corecollections.*`, but generated payload placement is still a separate boundary.

## Coupling summary after the move

- Concrete collection types depend on `core/types` runtime hooks for nil, errors, arity, printing/formatting, reduced handling, and registered type descriptors.
- The collection package does not import root `core`; `tests/layout_guard.sh` enforces this.
- Root runtime code imports `core/types/collections` directly where it constructs or inspects collection implementations.
- Proc registration, namespace mutation, evaluator internals, generated bootstrap placement, and sorted/transient runtime behavior remain the main blockers before further root shrinkage.

## Remaining extraction candidates

1. Runtime/env ownership: move environment/goroutine/concurrency runtime state only as a coherent batch, or first define a real runtime-facing contract for namespaces/procs/errors.
2. Generated/bootstrap boundary: continue updating generators so generated payloads do not instantiate root internals directly.
3. IR/WASM ownership: move IR model/executor clusters only after root runtime and collection dependencies are removed or routed through existing package contracts.

## Validation status

Targeted checks currently used for this boundary:

- `GOTMPDIR=$PWD/.cache/gotmp go test ./core ./core/types/collections -run '^$' -count=1`
- focused collection/reader/reduce/map/set/chunk contract tests under `./core`
- `GOTMPDIR=$PWD/.cache/gotmp go test ./core/types/collections -run '^Test' -count=1`
- `bash tests/layout_guard.sh .`
- gen-code compile and generated guards when bootstrap/generator files change
