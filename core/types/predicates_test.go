package types

import "testing"

func TestPredicates(t *testing.T) {
	if !IsSymbol(MakeSymbol(func(s string) *string { return &s }, "x")) {
		t.Fatal("symbol predicate failed")
	}
	if !IsKeyword(MakeKeyword(func(s string) *string { return &s }, "k")) {
		t.Fatal("keyword predicate failed")
	}
	if IsSeq(MakeString("x")) || IsVector(MakeString("x")) {
		t.Fatal("string should not satisfy seq/vector predicates")
	}
}
