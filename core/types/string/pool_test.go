package string

import (
	"sync"
	"testing"
)

func TestInternReusesStringPointer(t *testing.T) {
	p := Pool{}
	a := p.Intern("hello")
	b := p.Intern("hello")
	if a != b {
		t.Fatal("Intern should reuse pointer for identical strings")
	}
}

func TestInternIsConcurrent(t *testing.T) {
	p := Pool{}
	const goroutines = 32
	results := make(chan *string, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			results <- p.Intern("shared")
		}()
	}
	wg.Wait()
	close(results)
	var first *string
	for result := range results {
		if first == nil {
			first = result
		} else if result != first {
			t.Fatal("Intern returned different pointers concurrently")
		}
	}
}
