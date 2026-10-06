//go:build amd64 || arm64

package wasm

import (
	"context"
	"testing"
)

func TestNativeCompilerOnSupportedHost(t *testing.T) {
	rt, engine, err := NewRuntime(context.Background(), "compiler")
	if err != nil {
		t.Fatalf("explicit native compiler must be validated on this host: %v", err)
	}
	if engine != "compiler" {
		t.Fatal("native selection silently fell back", engine)
	}
	if err := rt.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
