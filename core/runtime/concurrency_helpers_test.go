package runtime

import (
	"testing"
	"time"
)

func TestCheckedMillisecondDuration(t *testing.T) {
	if got := CheckedMillisecondDuration(25, "timeout", func(msg string) any { return msg }); got != 25*time.Millisecond {
		t.Fatalf("duration = %s, want 25ms", got)
	}
	defer func() {
		if r := recover(); r != "timeout requires a non-negative millisecond value" {
			t.Fatalf("panic = %v, want non-negative millisecond error", r)
		}
	}()
	_ = CheckedMillisecondDuration(-1, "timeout", func(msg string) any { return msg })
}

func TestRunParallelCompletesAndPropagatesPanic(t *testing.T) {
	seen := make(chan int, 3)
	if r, panicked := RunParallel(3, nil, nil, func(i int) { seen <- i }); panicked || r != nil {
		t.Fatalf("RunParallel normal = (%v, %v), want (nil, false)", r, panicked)
	}
	close(seen)
	count := 0
	for range seen {
		count++
	}
	if count != 3 {
		t.Fatalf("run count = %d, want 3", count)
	}

	if r, panicked := RunParallel(2, nil, nil, func(i int) {
		if i == 1 {
			panic("boom")
		}
	}); !panicked || r != "boom" {
		t.Fatalf("RunParallel panic = (%v, %v), want (boom, true)", r, panicked)
	}
}

func TestRunParallelWaitsForAfterAndRecoversItsPanic(t *testing.T) {
	afterDone := make(chan struct{}, 1)
	if r, panicked := RunParallel(1, nil, func() { afterDone <- struct{}{} }, func(int) {}); panicked || r != nil {
		t.Fatalf("RunParallel after = (%v, %v), want (nil, false)", r, panicked)
	}
	select {
	case <-afterDone:
	default:
		t.Fatal("RunParallel returned before after completed")
	}

	if r, panicked := RunParallel(1, nil, func() { panic("after boom") }, func(int) {}); !panicked || r != "after boom" {
		t.Fatalf("RunParallel after panic = (%v, %v), want (after boom, true)", r, panicked)
	}
}
