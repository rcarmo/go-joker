# Direct runtime comparison

Generated from `benchmark-history.json` (Joker) and cross-language runtime scripts. Values are ms/op; lower is better.

| Benchmark | Joker | Python | Bun/corecollections.Node | Goja | let-go | Winner |
|---|---:|---:|---:|---:|---:|---|
| arithmetic-loop | 0.467 | 5.37 | 0.200 | 10.5 | 8.16 | Bun/corecollections.Node |
| recursive-fib | 35.1 | 10.0 | 0.790 | 40.1 | 25.9 | Bun/corecollections.Node |
| tail-recursive-sum | 0.083 | 3.83 | 0.230 | 6.86 | 4.98 | Joker |
| map-update-loop | 0.002 | 0.270 | 0.150 | 0.990 | 1.79 | Joker |
| word-frequency | 0.356 | 0.470 | 0.190 | 1.07 | 19.1 | Bun/corecollections.Node |
| nbody | 0.005 | 0.460 | 0.230 | 2.71 | 1.66 | Joker |
| spectral-norm | 0.096 | 12.1 | 0.710 | 37.0 | 27.5 | Joker |
| binary-trees | 3.26 | 20.5 | 3.60 | 99.7 | 84.9 | Joker |
| fannkuch | 0.140 | 7.58 | 0.540 | 15.6 | 13.4 | Joker |
| mandelbrot | 0.077 | 5.56 | 0.280 | 17.3 | 8.19 | Joker |
| fasta | 0.259 | 0.090 | 0.020 | 0.230 | 0.150 | Bun/corecollections.Node |
| knucleotide | 0.008 | 0.050 | 0.070 | 0.240 | 0.220 | Joker |
| reverse-complement | 0.000 | 0.010 | 0.040 | 0.050 | 0.110 | Joker |
| regex-redux | 0.058 | 0.100 | 0.040 | 0.110 | 0.070 | Bun/corecollections.Node |
| pidigits | 4.06 | 0.070 | 0.110 | 0.190 | 0.140 | Python |
