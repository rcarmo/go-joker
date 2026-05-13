package runtime

import (
	sdkruntime "runtime"
	"strconv"
)

// GoID extracts the current goroutine ID from the stack header.
// It is intended for cold-path runtime bookkeeping only.
func GoID() int64 {
	var buf [64]byte
	n := sdkruntime.Stack(buf[:], false)
	i := 10 // len("goroutine ")
	j := i
	for j < n && buf[j] >= '0' && buf[j] <= '9' {
		j++
	}
	id, _ := strconv.ParseInt(string(buf[i:j]), 10, 64)
	return id
}
