package string

import "testing"

func TestParseVersionTriplet(t *testing.T) {
	major, minor, incremental := ParseVersionTriplet("v1.2.3")
	if major != 1 || minor != 2 || incremental != 3 {
		t.Fatalf("ParseVersionTriplet returned %d.%d.%d", major, minor, incremental)
	}

	major, minor, incremental = ParseVersionTriplet("v1.bad.3")
	if major != 1 || minor != 0 || incremental != 3 {
		t.Fatalf("ParseVersionTriplet malformed returned %d.%d.%d", major, minor, incremental)
	}
}
