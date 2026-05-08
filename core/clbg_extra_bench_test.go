package core

import "testing"

func BenchmarkCLBGFannkuchRedux(b *testing.B) {
	expr := compileBenchExpr(b, fannkuchScript)
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
	expr := compileBenchExpr(b, mandelbrotScript)
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
	expr := compileBenchExpr(b, fastaScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGPidigits(b *testing.B) {
	expr := compileBenchExpr(b, pidigitsScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGKnucleotide(b *testing.B) {
	expr := compileBenchExpr(b, knucleotideScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGKnucleotideBestJoker(b *testing.B) {
	expr := compileBenchExpr(b, knucleotideBestJokerScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGReverseComplement(b *testing.B) {
	expr := compileBenchExpr(b, reverseComplementScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGReverseComplementBestJoker(b *testing.B) {
	expr := compileBenchExpr(b, reverseComplementBestJokerScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGRegexRedux(b *testing.B) {
	expr := compileBenchExpr(b, regexReduxScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCLBGRegexReduxBestJoker(b *testing.B) {
	expr := compileBenchExpr(b, regexReduxBestJokerScript)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Eval(expr, nil)
	}
}
