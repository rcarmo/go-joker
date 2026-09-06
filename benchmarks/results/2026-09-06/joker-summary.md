| Benchmark | runs | median ms/op | min ms | max ms | stdev % | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|
| `BenchmarkCLBGBinaryTrees` | 5 | 76.326 | 73.958 | 77.361 | 2.0 | 38024022 | 1268083 |
| `BenchmarkCLBGBinaryTreesBestJoker` | 5 | 3.259 | 3.144 | 3.267 | 1.7 | 2850857 | 178179 |
| `BenchmarkCLBGBinaryTreesParallel` | 5 | 48.897 | 46.378 | 52.778 | 4.7 | 76084725 | 2536481 |
| `BenchmarkCLBGFannkuchRedux` | 5 | 250.070 | 241.948 | 323.377 | 13.5 | 31213462 | 789681 |
| `BenchmarkCLBGFannkuchReduxBestJoker` | 5 | 0.140 | 0.137 | 0.141 | 1.3 | 216 | 5 |
| `BenchmarkCLBGFasta` | 5 | 0.259 | 0.250 | 0.263 | 1.8 | 199285 | 14844 |
| `BenchmarkCLBGKnucleotide` | 5 | 0.280 | 0.274 | 0.319 | 6.6 | 35025 | 2288 |
| `BenchmarkCLBGKnucleotideBestJoker` | 5 | 0.008 | 0.007 | 0.009 | 5.6 | 19736 | 10 |
| `BenchmarkCLBGMandelbrot` | 5 | 4.401 | 4.286 | 4.482 | 1.6 | 181951 | 10411 |
| `BenchmarkCLBGMandelbrotBestJoker` | 5 | 0.077 | 0.075 | 0.080 | 3.1 | 40 | 2 |
| `BenchmarkCLBGNBody` | 5 | 294.214 | 277.579 | 354.775 | 10.6 | 18888200 | 481180 |
| `BenchmarkCLBGNBodyBestJoker` | 5 | 0.005 | 0.004 | 0.006 | 13.3 | 24 | 2 |
| `BenchmarkCLBGPidigits` | 5 | 4.063 | 4.008 | 4.175 | 1.5 | 599531 | 17250 |
| `BenchmarkCLBGRegexRedux` | 5 | 0.205 | 0.203 | 0.207 | 0.9 | 53952 | 779 |
| `BenchmarkCLBGRegexReduxBestJoker` | 5 | 0.058 | 0.056 | 0.058 | 1.7 | 41368 | 332 |
| `BenchmarkCLBGReverseComplement` | 5 | 0.058 | 0.057 | 0.062 | 3.8 | 28808 | 576 |
| `BenchmarkCLBGReverseComplementBestJoker` | 5 | 0.000 | 0.000 | 0.000 | 1.0 | 224 | 2 |
| `BenchmarkCLBGSpectralNorm` | 5 | 30.233 | 29.856 | 31.517 | 2.2 | 5821220 | 153212 |
| `BenchmarkCLBGSpectralNormBestJoker` | 5 | 0.096 | 0.095 | 0.113 | 8.1 | 1272 | 5 |
| `BenchmarkEvalArithmeticLoop` | 5 | 0.467 | 0.453 | 0.473 | 1.9 | 226 | 4 |
| `BenchmarkEvalMapUpdateLoop` | 5 | 0.929 | 0.924 | 1.011 | 4.0 | 288946 | 15704 |
| `BenchmarkEvalMapUpdateLoopBestJoker` | 5 | 0.002 | 0.002 | 0.002 | 2.7 | 24 | 2 |
| `BenchmarkEvalRecursiveFib` | 5 | 35.081 | 34.902 | 36.671 | 2.1 | 7211289 | 450872 |
| `BenchmarkEvalTailRecursiveSum` | 5 | 4.818 | 4.684 | 5.037 | 3.0 | 291 | 8 |
| `BenchmarkEvalTailRecursiveSumLoopRecur` | 5 | 0.083 | 0.081 | 0.090 | 4.3 | 217 | 4 |
| `BenchmarkEvalWordFrequency` | 5 | 0.356 | 0.341 | 0.359 | 2.0 | 769858 | 8074 |
