# benchmarks/compare

This sub-project provides a direct runtime comparison workflow using the same cross-language benchmark ports:

- Joker (from `benchmarks/benchmark-history.json`, current series)
- Python (`benchmarks/cross_lang_bench.py`)
- Bun/Node (`benchmarks/cross_lang_bench.js`)
- Goja (`benchmarks/cross_lang_bench_goja.go`)
- let-go (`benchmarks/cross_lang_bench.clj`)

## Usage

From repo root:

```bash
make compare-bench
```

Outputs are written to:

- `benchmarks/compare/out/latest/*.txt` (raw runtime outputs)
- `benchmarks/compare/out/latest/direct-comparison.md` (combined table)

You can override output directory:

```bash
make compare-bench COMPARE_OUT=benchmarks/compare/out/run-$(date +%Y%m%d-%H%M%S)
```

## Notes

- Missing runtimes are handled gracefully and shown as `-` in the final table.
- `let-go` command detection supports both `lg` and `let-go` binaries.
