//go:build gen_code
// +build gen_code

package main

import "testing"

func TestCoreTypeStringMapsMovedRuntimeTypes(t *testing.T) {
	tests := map[string]string{
		"Atom":      "corert.Atom",
		"*Atom":     "*corert.Atom",
		"[]Atom":    "[]corert.Atom",
		"Channel":   "corert.ObjectChannel",
		"*Channel":  "*corert.ObjectChannel",
		"[]Channel": "[]corert.ObjectChannel",
	}
	for input, want := range tests {
		if got := coreTypeString(input); got != want {
			t.Fatalf("coreTypeString(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCoreTypeStringMapsMovedCollectionTypes(t *testing.T) {
	tests := map[string]string{
		"ArrayMap":           "corecollections.ArrayMap",
		"*ArrayMap":          "*corecollections.ArrayMap",
		"[]PersistentVector": "[]corecollections.PersistentVector",
	}
	for input, want := range tests {
		if got := coreTypeString(input); got != want {
			t.Fatalf("coreTypeString(%q) = %q, want %q", input, got, want)
		}
	}
}
