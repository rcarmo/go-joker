package core_test

import (
	"strconv"
	"sync/atomic"
	"testing"

	corestr "github.com/rcarmo/go-joker/core/types/string"
)

func BenchmarkStringPoolHotReadParallel(b *testing.B) {
	pool := corestr.Pool{}
	pool.Intern("shared")
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.Intern("shared")
		}
	})
}

func BenchmarkStringPoolDistinctPoolsParallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		pool := corestr.Pool{}
		pool.Intern("shared")
		for pb.Next() {
			pool.Intern("shared")
		}
	})
}

func BenchmarkStringPoolWriteParallel(b *testing.B) {
	pool := corestr.Pool{}
	var next atomic.Int64
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := next.Add(1)
			pool.Intern(strconv.FormatInt(n, 10))
		}
	})
}
