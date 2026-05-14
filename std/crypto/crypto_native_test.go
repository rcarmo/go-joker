package crypto

import (
	"encoding/hex"
	"testing"
)

func TestHmacSumSHA256Boundary(t *testing.T) {
	got := hex.EncodeToString([]byte(hmacSum(":sha256", "hello", "secret")))
	const want = "88aab3ede8d3adf94d26ab90d3bafd4a2083070c3bcce9c014ee04a443847c0b"
	if got != want {
		t.Fatalf("hmacSum(:sha256) = %s, want %s", got, want)
	}
}
