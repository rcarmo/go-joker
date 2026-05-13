package string

import "testing"

func TestJoinDotted(t *testing.T) {
	if got := JoinDotted([]string{"Math", "PI"}); got != "Math.PI" {
		t.Fatalf("JoinDotted() = %q", got)
	}
}
