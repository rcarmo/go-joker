# Joker benchmark artifacts

This directory stores the benchmark dataset and generated chart for the Joker optimization work.

## Files

- `benchmark-history.json` — source-of-truth dataset for published charts
- `benchmark-cross-language.svg` — latest cross-language matrix chart
- `benchmark-improvements.svg` — generated SVG chart vs. original Joker
- `generate_svg.go` — tiny generator that reads the JSON and writes the improvement SVG
- `run_benchmarks.py` — repeat-run median harness for lower-noise decisions

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
2. Run the full 5x matrix command when publishing a checkpoint.
3. Copy selected checkpoint values into `benchmark-history.json`.
4. Regenerate `benchmark-improvements.svg` with `generate_svg.go` and update `benchmark-cross-language.svg`.
5. Commit the JSON and SVG files together.
