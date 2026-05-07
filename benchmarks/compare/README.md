# benchmarks/compare

This sub-project provides two comparison workflows:

1. Cross-language benchmark ports:
   - Joker (from `benchmarks/benchmark-history.json`, current series)
   - Python (`benchmarks/cross_lang_bench.py`)
   - Bun/Node (`benchmarks/cross_lang_bench.js`)
   - Goja (`benchmarks/cross_lang_bench_goja.go`)
   - let-go (`benchmarks/cross_lang_bench.clj`)

2. let-go benchmark suite parity run (mirrored from `nooga/let-go/benchmark`):
   - `fib`, `loop-recur`, `map-filter`, `persistent-map`, `reduce`, `tak`, `transducers`
   - compared across available Clojure runtimes (`let-go`, `go-joker`, and optionally `joker`/`babashka`/`clj`)

## Usage

From repo root:

```bash
make compare-bench
```

Outputs are written to:

- `benchmarks/compare/out/latest/*.txt` (raw cross-language runtime outputs)
- `benchmarks/compare/out/latest/direct-comparison.md` (cross-language combined table)
- `benchmarks/compare/out/latest/letgo-suite-comparison.md` (let-go suite parity table)
- `benchmarks/compare/out/latest/letgo-suite-results.json` (raw stats for let-go suite parity run)

You can override output directory:

```bash
make compare-bench COMPARE_OUT=benchmarks/compare/out/run-$(date +%Y%m%d-%H%M%S)
```

## Notes

- Missing runtimes are handled gracefully and shown as `-` in the final tables.
- `let-go` command detection supports both `lg` and `let-go` binaries.
- For the let-go parity suite, runtime binaries can be overridden with env vars:
  - `LETGO_BIN=/path/to/let-go`
  - `GOJOKER_BIN=/path/to/go-joker`
  - `JOKER_BIN=/path/to/joker`
  - `BB_BIN=/path/to/bb`
  - `CLJ_BIN=/path/to/clj`
- The parity runner attempts all mirrored workloads and reports failures with reasons in the runtime notes section.
