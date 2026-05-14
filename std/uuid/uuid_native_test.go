package uuid

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestNewUUIDSetsVersionAndVariant(t *testing.T) {
	oldRander := rander
	t.Cleanup(func() { rander = oldRander })
	rander = bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})

	got := new()
	const want = "00010203-0405-4607-8809-0a0b0c0d0e0f"
	if got != want {
		t.Fatalf("new UUID = %s, want %s", got, want)
	}
}

func TestUUIDDefaultRanderIsCryptoRand(t *testing.T) {
	if rander != rand.Reader {
		t.Fatal("default UUID rander should be crypto/rand.Reader")
	}
}
