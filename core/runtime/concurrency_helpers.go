package runtime

import "time"

const maxMillisecondDuration = int64(1<<63-1) / int64(time.Millisecond)

func CheckedMillisecondDuration(ms int, context string, errf func(string) any) time.Duration {
	if ms < 0 {
		panic(errf(context + " requires a non-negative millisecond value"))
	}
	if int64(ms) > maxMillisecondDuration {
		panic(errf(context + " millisecond value is too large"))
	}
	return time.Duration(ms) * time.Millisecond
}

func RunParallel(count int, before func(), after func(), run func(int)) (any, bool) {
	done := make(chan int, count)
	panicCh := make(chan any, count)
	for i := 0; i < count; i++ {
		go func(idx int) {
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
				done <- idx
			}()
			if before != nil {
				before()
			}
			if after != nil {
				defer after()
			}
			run(idx)
		}(i)
	}
	for i := 0; i < count; i++ {
		<-done
	}
	select {
	case r := <-panicCh:
		return r, true
	default:
		return nil, false
	}
}
