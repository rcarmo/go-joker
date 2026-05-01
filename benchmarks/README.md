# Joker benchmark artifacts

This directory stores the benchmark dataset and generated chart for the Joker optimization work.

## Files

- `benchmark-history.json` — source-of-truth dataset
- `benchmark-improvements.svg` — generated SVG chart
- `generate_svg.go` — tiny generator that reads the JSON and writes the SVG

## Regenerate the chart

From `third_party/joker/`:

```bash
go run ./benchmarks/generate_svg.go ./benchmarks
```

## Stable benchmark harness

Use the median harness when comparing optimization decisions. It runs the same Go benchmark command repeatedly, parses `ns/op`, `B/op`, and `allocs/op`, and prints a Markdown table plus optional JSON.

```bash
python3 benchmarks/run_benchmarks.py \
  --runs 7 \
  --bench 'BenchmarkCLBG|BenchmarkEval|BenchmarkWasm' \
  --benchtime 5x \
  --json /tmp/joker-bench.json

# Focused string/typed-IR work
python3 benchmarks/run_benchmarks.py \
  --runs 9 \
  --bench 'BenchmarkIRString|BenchmarkIRChar|BenchmarkCLBGReverseComplement|BenchmarkCLBGKnucleotide' \
  --benchtime 20x
```

Use `--env JOKER_WASM_MULTIFN=force` to probe the multi-function WASM path separately from the default strategy.

## Update workflow

1. Run the median harness for decision-making.
2. If updating published charts, copy the selected median values into `benchmark-history.json`.
3. Regenerate the SVG with `generate_svg.go`.
4. Commit both files.
