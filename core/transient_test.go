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

func TestTransientMapStringKeys(t *testing.T) {
	tm := MapToTransient(nil)
	tm.AssocInPlace(String{S: "alpha"}, Int{I: 1})
	tm.AssocInPlace(String{S: "beta"}, Int{I: 2})
	tm.AssocInPlace(String{S: "alpha"}, Int{I: 3})
	if tm.Count() != 2 {
		t.Fatalf("expected 2, got %d", tm.Count())
	}
	ok, v := tm.Get(String{S: "alpha"})
	if !ok || v.(Int).I != 3 {
		t.Fatalf("expected 3 for alpha")
	}
	pm := tm.ToPersistent().(Map)
	ok, v = pm.Get(String{S: "beta"})
	if !ok || v.(Int).I != 2 {
		t.Fatalf("expected persistent beta=2")
	}
}

func TestTransientVectorProcs(t *testing.T) {
	vec := NewArrayVectorFrom(Int{I: 1}, Int{I: 2})
	tv, ok := procTransient([]Object{vec}).(*TransientVector)
	if !ok {
		t.Fatalf("transient vector proc returned %T", tv)
	}
	if got := procIsTransient([]Object{tv}); !got.Equals(Boolean{B: true}) {
		t.Fatalf("transient? returned %s", got.ToString(false))
	}
	if procAssocBang([]Object{tv, Int{I: 1}, Int{I: 20}}) != tv {
		t.Fatal("assoc! should return the same transient vector")
	}
	if procConjBang([]Object{tv, Int{I: 3}}) != tv {
		t.Fatal("conj! should return the same transient vector")
	}
	if tv.Count() != 3 || !tv.At(1).Equals(Int{I: 20}) || !tv.At(2).Equals(Int{I: 3}) {
		t.Fatalf("unexpected transient vector state: count=%d", tv.Count())
	}
	if procPopBang([]Object{tv}) != tv {
		t.Fatal("pop! should return the same transient vector")
	}
	persisted := procPersistentBang([]Object{tv}).(*ArrayVector)
	if persisted.Count() != 2 || !persisted.At(1).Equals(Int{I: 20}) {
		t.Fatalf("unexpected persistent vector: %s", persisted.ToString(false))
	}
}

func TestTransientMapProcs(t *testing.T) {
	tm, ok := procTransient([]Object{EmptyArrayMap()}).(*TransientMap)
	if !ok {
		t.Fatalf("transient map proc returned %T", tm)
	}
	if got := procIsTransient([]Object{tm}); !got.Equals(Boolean{B: true}) {
		t.Fatalf("transient? returned %s", got.ToString(false))
	}
	if procAssocBang([]Object{tm, MakeKeyword("a"), Int{I: 1}}) != tm {
		t.Fatal("assoc! should return the same transient map")
	}
	if procConjBang([]Object{tm, String{S: "b"}, Int{I: 2}}) != tm {
		t.Fatal("conj! should return the same transient map")
	}
	persisted := procPersistentBang([]Object{tm}).(Map)
	if persisted.Count() != 2 {
		t.Fatalf("persistent map count = %d", persisted.Count())
	}
	if ok, got := persisted.Get(String{S: "b"}); !ok || !got.Equals(Int{I: 2}) {
		t.Fatalf("missing persisted string key: %v %v", ok, got)
	}
}

func TestIRTransientStringBuilder(t *testing.T) {
	t.Setenv("JOKER_IR_STRING_BUILDER", "force")
	requireString(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      s
      (recur (inc i) (str s (nth dna i))))))`), "ACGT")
}

func TestIRTransientStringPrependAuto(t *testing.T) {
	requireString(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      s
      (recur (inc i) (str (nth dna i) s)))))`), "TGCA")
}
