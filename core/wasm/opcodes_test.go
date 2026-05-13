package wasm

import "testing"

func TestValueTypeConstants(t *testing.T) {
	cases := map[string]byte{"i32": ValTypeI32, "i64": ValTypeI64, "f32": ValTypeF32, "f64": ValTypeF64}
	want := map[string]byte{"i32": 0x7f, "i64": 0x7e, "f32": 0x7d, "f64": 0x7c}
	for k, v := range cases {
		if v != want[k] {
			t.Fatalf("%s value type = %#x, want %#x", k, v, want[k])
		}
	}
}
