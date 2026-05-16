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
	i := len("goroutine ")
	if n <= i {
		return 0
	}
	j := i
	for j < n && buf[j] >= '0' && buf[j] <= '9' {
		j++
	}
	if j == i {
		return 0
	}
	id, err := strconv.ParseInt(string(buf[i:j]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
