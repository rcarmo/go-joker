package string

import "testing"

func TestParseVersionTriplet(t *testing.T) {
	major, minor, incremental := ParseVersionTriplet("v1.2.3")
	if major != 1 || minor != 2 || incremental != 3 {
		t.Fatalf("ParseVersionTriplet returned %d.%d.%d", major, minor, incremental)
	}
}
