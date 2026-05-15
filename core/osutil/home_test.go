package osutil

import "testing"

func TestHomeDirPrefersHomeEnv(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("USERPROFILE", "/users/test")
	if got := HomeDir(); got != "/home/test" {
		t.Fatalf("HomeDir() = %q, want HOME", got)
	}
}

func TestHomeDirFallsBackToUserProfile(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "/users/test")
	if got := HomeDir(); got != "/users/test" {
		t.Fatalf("HomeDir() = %q, want USERPROFILE", got)
	}
}
