package core_test

import (
	. "github.com/rcarmo/go-joker/core"
	"math"
	"testing"

	"github.com/rcarmo/go-joker/tests/clbgscripts"
)

func requireBenchInt(t *testing.T, script string, want int) {
	t.Helper()
	got, ok := Eval(compileBenchExpr(t, script), nil).(Int)
	if !ok {
		t.Fatalf("%s returned non-Int %T", script, got)
	}
	if got.I != want {
		t.Fatalf("%s = %d, want %d", script, got.I, want)
	}
}

func requireBenchDouble(t *testing.T, script string, want, tolerance float64) {
	t.Helper()
	got, ok := Eval(compileBenchExpr(t, script), nil).(Double)
	if !ok {
		t.Fatalf("%s returned non-Double %T", script, got)
	}
	if math.Abs(got.D-want) > tolerance {
		t.Fatalf("%s = %.17g, want %.17g ± %.1g", script, got.D, want, tolerance)
	}
}

func TestBenchmarkPortableResults(t *testing.T) {
	clbgInit()
	tests := []struct {
		name   string
		script string
		want   int
	}{
		{"mandelbrot", clbgscripts.MandelbrotScript, 633},
		{"binary-trees", binaryTreesScript, 358401},
		{"fannkuch", clbgscripts.FannkuchScript, 16228},
		{"fasta", clbgscripts.FastaScript, 150034},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { requireBenchInt(t, tt.script, tt.want) })
	}
	t.Run("spectral-norm", func(t *testing.T) { requireBenchDouble(t, spectralNormScript, 1.2741938369830932, 1e-12) })
	t.Run("nbody", func(t *testing.T) { requireBenchDouble(t, nbodyScript, 0.5417510901232987, 1e-12) })
	t.Run("pidigits", func(t *testing.T) { requireBenchDouble(t, clbgscripts.PidigitsScript, 9.855316369115884e+11, 1e-3) })
}

func TestBenchmarkMicroResults(t *testing.T) {
	requireBenchInt(t, `(loop [i 0 s 0]
  (if (= i 100000)
    s
    (recur (inc i) (+ s (rem (* i 7) 11)))))`, 500001)
	requireBenchInt(t, `(letfn [(fib [n] (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2)))))]
  (loop [i 3 s 0]
    (if (zero? i)
      s
      (recur (dec i) (+ s (fib 24))))))`, 139104)
	requireBenchInt(t, `(loop [n 100000 acc 0]
  (if (zero? n)
    acc
    (recur (dec n) (+ acc n))))`, 5000050000)
	requireBenchInt(t, `(bench-map-update-loop 5000)`, 938)
	requireBenchInt(t, `(let [text "alpha beta gamma delta epsilon zeta eta theta alpha beta gamma delta epsilon zeta eta theta"
      freqs (frequencies (split-whitespace text))]
  (+ (get freqs "theta" 0) (get freqs "alpha" 0)))`, 4)
}

func TestBenchmarkBestJokerResults(t *testing.T) {
	clbgInit()
	tests := []struct {
		name   string
		script string
		want   int
	}{
		{"mandelbrot", `(bench-mandelbrot-count 40 50)`, 633},
		{"binary-trees", `(bench-binary-trees 14)`, 358401},
		{"fannkuch", `(bench-fannkuch 7)`, 16228},
		{"map-update-loop", `(bench-map-update-loop 5000)`, 938},
		{"knucleotide", clbgscripts.KNucleotideBestJokerScript, 27},
		{"reverse-complement", clbgscripts.ReverseComplementBestJokerScript, 196},
		{"regex-redux", clbgscripts.RegexReduxBestJokerScript, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { requireBenchInt(t, tt.script, tt.want) })
	}
	t.Run("spectral-norm", func(t *testing.T) { requireBenchDouble(t, `(bench-spectral-norm 50)`, 1.2741938369830932, 1e-12) })
	t.Run("nbody", func(t *testing.T) { requireBenchDouble(t, `(bench-nbody-energy 100)`, -0.16926665164096838, 1e-12) })
}
