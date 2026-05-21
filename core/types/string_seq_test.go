package types

import "testing"

func TestStringSeqDescriptor(t *testing.T) {
	seq := &StringSeq{S: "abc", Off: 1}
	if seq.S != "abc" || seq.Off != 1 {
		t.Fatalf("StringSeq = %#v", seq)
	}
}
