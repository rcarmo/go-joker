package string

import "testing"

func TestIsJokerdPath(t *testing.T) {
	if !IsJokerdPath("/tmp/.jokerd/cache") {
		t.Fatal("expected .jokerd path to be detected")
	}
	if IsJokerdPath("/tmp/work") {
		t.Fatal("did not expect regular path to be detected")
	}
}
