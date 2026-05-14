package core_test

import (
	. "github.com/rcarmo/go-joker/core"
	"testing"

	"github.com/rcarmo/go-joker/tests/clbgscripts"
)

func BenchmarkCLBGFannkuchRedux(b *testing.B) {
	expr := compileBenchExpr(b, clbgscripts.FannkuchScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGFannkuchReduxBestJoker(b *testing.B) {
	expr := compileBenchExpr(b, `(bench-fannkuch 7)`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGMandelbrot(b *testing.B) {
	expr := compileBenchExpr(b, clbgscripts.MandelbrotScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGMandelbrotBestJoker(b *testing.B) {
	expr := compileBenchExpr(b, `(bench-mandelbrot-count 40 50)`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGFasta(b *testing.B) {
	expr := compileBenchExpr(b, clbgscripts.FastaScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGPidigits(b *testing.B) {
	expr := compileBenchExpr(b, clbgscripts.PidigitsScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGKnucleotide(b *testing.B) {
	expr := compileBenchExpr(b, clbgscripts.KNucleotideScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGKnucleotideBestJoker(b *testing.B) {
	expr := compileBenchExpr(b, clbgscripts.KNucleotideBestJokerScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGReverseComplement(b *testing.B) {
	expr := compileBenchExpr(b, clbgscripts.ReverseComplementScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGReverseComplementBestJoker(b *testing.B) {
	expr := compileBenchExpr(b, clbgscripts.ReverseComplementBestJokerScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGRegexRedux(b *testing.B) {
	expr := compileBenchExpr(b, clbgscripts.RegexReduxScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGRegexReduxBestJoker(b *testing.B) {
	expr := compileBenchExpr(b, clbgscripts.RegexReduxBestJokerScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}
