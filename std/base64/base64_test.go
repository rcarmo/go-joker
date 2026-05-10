package base64

import "testing"

func TestBase64EncodeDecodeString(t *testing.T) {
	encoded := encodeString("hello, joker")
	if encoded != "aGVsbG8sIGpva2Vy" {
		t.Fatalf("unexpected encoding: %s", encoded)
	}
	if decoded := decodeString(encoded); decoded != "hello, joker" {
		t.Fatalf("unexpected decoding: %s", decoded)
	}
}
