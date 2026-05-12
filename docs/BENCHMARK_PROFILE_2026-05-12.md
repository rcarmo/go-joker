# Benchmark/profile audit — 2026-05-12

This records the full benchmark/profile pass run after the runtime/refactor guardrail work.

## Commands

```bash
export PATH=/workspace/.cache/go-install/go/bin:/home/agent/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.2.linux-amd64/bin:$PATH

TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  python3 benchmarks/run_benchmarks.py \
    --runs 3 \
    --bench 'BenchmarkCLBG|BenchmarkEval|BenchmarkWasm' \
    --benchtime 5x \
    --timeout 900s \
    --json /workspace/tmp/go-joker-bench-profile/benchmark-summary.json \
    --raw-dir /workspace/tmp/go-joker-bench-profile/raw

TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core ./std/... -run '^$' -bench . -benchmem -benchtime=3x -timeout=1200s

TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core -run '^$' -bench 'BenchmarkCLBG|BenchmarkEval|BenchmarkWasm' \
    -benchmem -benchtime=5x -timeout=900s \
    -cpuprofile /workspace/tmp/go-joker-bench-profile/cpu.pprof \
    -memprofile /workspace/tmp/go-joker-bench-profile/mem.pprof

TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core -run '^$' -bench . -benchmem -benchtime=3x -timeout=1200s \
    -cpuprofile /workspace/tmp/go-joker-bench-profile/core-all.cpu.pprof \
    -memprofile /workspace/tmp/go-joker-bench-profile/core-all.mem.pprof
```

## CLBG/Eval/WASM median summary

3 runs, `-benchtime=5x`, i7-12700, Go 1.26.2 toolchain.

| Benchmark | median ms/op | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkCLBGNBody` | 844.238 | 182910161 | 2373207 |
| `BenchmarkCLBGNBodyBestJoker` | 0.005 | 25 | 2 |
| `BenchmarkCLBGSpectralNorm` | 71.653 | 18471788 | 285871 |
| `BenchmarkCLBGSpectralNormBestJoker` | 0.138 | 1273 | 5 |
| `BenchmarkCLBGBinaryTrees` | 138.722 | 39003320 | 1098755 |
| `BenchmarkCLBGBinaryTreesParallel` | 38.620 | 77933460 | 2196535 |
| `BenchmarkCLBGBinaryTreesBestJoker` | 5.570 | 2850857 | 178179 |
| `BenchmarkCLBGFannkuchRedux` | 310.344 | 78487081 | 1042176 |
| `BenchmarkCLBGFannkuchReduxBestJoker` | 0.229 | 217 | 5 |
| `BenchmarkCLBGMandelbrot` | 8.059 | 422320 | 5008 |
| `BenchmarkCLBGMandelbrotBestJoker` | 0.089 | 41 | 2 |
| `BenchmarkCLBGFasta` | 0.048 | 5756 | 37 |
| `BenchmarkCLBGPidigits` | 0.040 | 6425 | 35 |
| `BenchmarkCLBGKnucleotide` | 0.500 | 57643 | 2544 |
| `BenchmarkCLBGKnucleotideBestJoker` | 0.009 | 19736 | 10 |
| `BenchmarkCLBGReverseComplement` | 0.099 | 30408 | 592 |
| `BenchmarkCLBGReverseComplementBestJoker` | 0.001 | 224 | 2 |
| `BenchmarkCLBGRegexRedux` | 0.658 | 164040 | 1911 |
| `BenchmarkCLBGRegexReduxBestJoker` | 0.081 | 48643 | 332 |
| `BenchmarkEvalArithmeticLoop` | 0.286 | 5192 | 25 |
| `BenchmarkEvalRecursiveFib` | 1.085 | 8723 | 76 |
| `BenchmarkEvalTailRecursiveSum` | 13.331 | 3200080 | 299778 |
| `BenchmarkEvalTailRecursiveSumLoopRecur` | 0.086 | 5112 | 24 |
| `BenchmarkEvalWordFrequency` | 0.650 | 538232 | 8139 |
| `BenchmarkEvalMapUpdateLoop` | 1.465 | 129648 | 10727 |
| `BenchmarkEvalMapUpdateLoopBestJoker` | 0.002 | 25 | 2 |
| `BenchmarkWasmArithmeticLoop` | 0.324 | 473 | 5 |
| `BenchmarkWasmFloatLoop` | 0.113 | 473 | 5 |
| `BenchmarkWasmArrayF64Sum` | 0.023 | 0 | 0 |

## Profile findings

The profiled CLBG/Eval/WASM run and the full core benchmark profile agree on the main bottleneck: allocation-heavy interpreted/portable paths spend most sampled CPU time in Go GC scanning and heap metadata.

Top CPU profile shape from the full core benchmark profile:

- `runtime.scanobject`: 89.74% cumulative
- `runtime.gcDrain`: 92.17% cumulative
- `runtime.(*mspan).heapBitsSmallForAddr`: 29.16% flat
- `runtime.findObject`: 22.68% flat
- `core.irExec`: 1.11% cumulative in the full-core profile, higher in the CLBG/Eval/WASM-only profile
- `core.irExecTyped`: 4.32% cumulative in the full-core profile

Top allocation-space profile from the CLBG/Eval/WASM profile:

- `core.irCompileFnWithFrame`: 985.67MB cumulative
- `core.irExec`: 706.21MB cumulative
- `core.irCompileMultiArity`: 1161.69MB cumulative
- `core.newIRFrameStack`: 165.69MB flat
- `core/internal/ir.NewProgram`: 133.52MB flat

## Interpretation

- Best-Joker/native-shaped benchmarks remain fast and low allocation.
- Portable/interpreted CLBG shapes are dominated by allocation churn and follow-on GC scan cost.
- Near-term optimization should focus on reducing repeated IR compile/envelope allocation and frame-stack allocation in interpreted/portable loops before chasing opcode micro-optimizations. Initial follow-up addressed the frame-stack allocation path with bounded pools and fixed unstable IR function cache keys; keep profiling after each batch.
- `BenchmarkCLBGBinaryTreesParallel` is still useful as a separate concurrency smoke benchmark, but remains allocation-heavy and noisy.

## Follow-up optimization notes

After this audit, two low-risk allocation fixes were implemented and validated:

- boxed/typed IR self-call frame stacks now use bounded pools and clear retained object slots on release;
- IR function compilation now uses stable arity/variadic keys and has contract coverage to prevent repeated compile/envelope allocation regressions.

Focused smoke after those changes (`-benchtime=20x`) showed lower allocations on recursive/loop-recur paths:

| Benchmark | time | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkEvalRecursiveFib` | 1.106ms | 3378 | 40 |
| `BenchmarkEvalTailRecursiveSumLoopRecur` | 0.0777ms | 1737 | 11 |
| `BenchmarkFib20` | 3.088ms | 778 | 7 |
| `BenchmarkCLBGNBodyBestJoker` | 0.0086ms | 24 | 2 |

Keep this as a directional smoke result only; rerun the full profile before making broader claims.

## Artifacts

The raw benchmark/profile artifacts for this run were packaged as `go-joker-bench-profile.tar.gz` in the chat session that produced this report.
