package osutil

import "testing"

func TestHomeDirReturnsString(t *testing.T) {
	_ = HomeDir()
}
