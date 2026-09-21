package core_test

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corestr "github.com/rcarmo/go-joker/core/types/string"
	"strings"
	"sync/atomic"
	"testing"
)

// Fixed-iteration runs are required: each unique value is classified once so
// cache hits do not stand in for scanning. Keep sample sizes bounded.
var asciiSampleID atomic.Uint64

func BenchmarkASCIIColdClassification(b *testing.B) {
	for _, size := range []int{64, 1024, 16384} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			if b.N > 10000 {
				b.Skip("use -benchtime=1000x or another bounded fixed iteration count")
			}
			inputs := make([]string, b.N)
			prefix := fmt.Sprintf("sample-%d-", asciiSampleID.Add(1))
			for i := range inputs {
				inputs[i] = strings.Repeat("a", size) + fmt.Sprintf("%s%d", prefix, i)
			}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for _, s := range inputs {
				if !corestr.IsASCII(s) {
					b.Fatal("ASCII misclassified")
				}
			}
		})
	}
}

// Model scanning a fresh batch of ASCII log records through the public String
// count operation. Unique batches avoid cached classification between samples.
func BenchmarkASCIIRecordBatch(b *testing.B) {
	if b.N > 500 {
		b.Skip("use -benchtime=500x -count=1; repeat in fresh processes for allocation comparisons")
	}
	inputs := make([]string, b.N*128)
	prefix := fmt.Sprintf("batch-%d-", asciiSampleID.Add(1))
	expected := make([]int, b.N)
	for i := range inputs {
		inputs[i] = strings.Repeat("level=info message=request-complete ", 128) + fmt.Sprintf("%s%d", prefix, i)
		expected[i/128] += len(inputs[i])
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		total := 0
		for _, s := range inputs[i*128 : (i+1)*128] {
			total += (coretypes.String{S: s}).Count()
		}
		if total != expected[i] {
			b.Fatal("record length mismatch")
		}
	}
}
