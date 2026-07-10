# String-pool contention benchmark

Measured on 2026-07-10 with Go 1.26.5 on an Intel Core i7-12700. The command was:

```bash
go test ./benchmarks/core -run '^$' -bench '^BenchmarkStringPool' \
  -benchmem -benchtime=2s -count=5 -cpu=1,6
```

Representative medians for the current synchronized `core/types/string.Pool` were:

| Workload | 1 CPU | 6 CPUs |
| --- | ---: | ---: |
| Shared pool, existing string | 32.7 ns/op | 34.6 ns/op |
| Distinct pools, existing string | 32.6 ns/op | 39.2 ns/op |
| Shared pool, unique writes | 470 ns/op | 420 ns/op |

The production runtime has one long-lived pool (`core.STRINGS`); other `Pool` instances occur only in tests and generated bootstrap declarations refer to that same pool. The representative hot-read workload therefore scales from one to six CPUs with only a small change, while writes are uncommon after bootstrap.

## Decision

Keep the global `RWMutex` implementation for now. Replacing the map-compatible global lock would require either changing generated bootstrap literals or maintaining per-map lock identity through unsafe/runtime details. The measured production-shaped workload does not justify that complexity.

Reconsider this decision if profiles show meaningful time in `Pool.Intern`/`sync.(*RWMutex)` or if multiple production pools are introduced.
