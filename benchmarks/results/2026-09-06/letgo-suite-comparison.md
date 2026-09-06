# let-go benchmark suite comparison

Benchmarks mirrored from `nooga/let-go/benchmark` (`fib`, `loop-recur`, `map-filter`, `persistent-map`, `reduce`, `tak`, `transducers`).

Warmup: 3, timed runs: 10. Values are ms/op (lower is better).

| Benchmark | let-go | go-joker | Winner |
|---|---:|---:|---|
| fib | 1733.2 | 2134.1 | let-go |
| loop-recur | 48.6 | 8.84 | go-joker |
| map-filter | 3.17 | 6.02 | let-go |
| persistent-map | 15.0 | 16.9 | let-go |
| reduce | 67.2 | 6.87 | go-joker |
| tak | 1887.8 | 2637.2 | let-go |
| transducers | 2.84 | 6.05 | let-go |

## Runtime notes

- let-go: all benchmarks ran.
- go-joker: all benchmarks ran.
