package core

import "testing"

func TestTransientVector(t *testing.T) {
	v := &ArrayVector{arr: []Object{Int{I: 1}, Int{I: 2}, Int{I: 3}}}
	tv := ToTransient(v)
	tv.AssocInPlace(Int{I: 1}, Int{I: 99})
	tv.ConjInPlace(Int{I: 4})
	if tv.Count() != 4 {
		t.Fatalf("expected count 4, got %d", tv.Count())
	}
	if tv.Nth(1).(Int).I != 99 {
		t.Fatalf("expected 99 at index 1")
	}
	pv := tv.ToPersistent()
	if pv.Count() != 4 {
		t.Fatalf("persistent count wrong")
	}
}

func TestTransientMap(t *testing.T) {
	m := EmptyArrayMap()
	m.Add(MakeKeyword("a"), Int{I: 1})
	m.Add(MakeKeyword("b"), Int{I: 2})
	tm := MapToTransient(m)
	tm.AssocInPlace(MakeKeyword("c"), Int{I: 3})
	tm.AssocInPlace(MakeKeyword("a"), Int{I: 99})
	if tm.Count() != 3 {
		t.Fatalf("expected 3, got %d", tm.Count())
	}
	ok, v := tm.Get(MakeKeyword("a"))
	if !ok || v.(Int).I != 99 {
		t.Fatalf("expected 99 for :a")
	}
	pm := tm.ToPersistent()
	if pm == nil {
		t.Fatal("persistent returned nil")
	}
}

func TestTransientProcs(t *testing.T) {
	t.Skip("transient procs registered but parser symbol resolution needs core.joke wrappers")
}

func TestTransientMapProcs(t *testing.T) {
	t.Skip("transient procs registered but parser symbol resolution needs core.joke wrappers")
}

func BenchmarkTransientVectorLoop(b *testing.B) {
	b.Skip("transient procs need core.joke wrappers for parser resolution")
}
