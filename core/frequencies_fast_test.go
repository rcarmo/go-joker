package core

import "testing"

func TestFrequenciesFastStringSeq(t *testing.T) {
	res := evalTestScript(t, `(frequencies ["alpha" "beta" "alpha" "theta" "beta" "alpha"])`)
	m, ok := res.(Map)
	if !ok {
		t.Fatalf("expected map, got %T", res)
	}
	ok, v := m.Get(String{S: "alpha"})
	if !ok || v.(Int).I != 3 {
		t.Fatalf("expected alpha=3, got %v %v", ok, v)
	}
	ok, v = m.Get(String{S: "theta"})
	if !ok || v.(Int).I != 1 {
		t.Fatalf("expected theta=1, got %v %v", ok, v)
	}
}

func TestSplitWhitespace(t *testing.T) {
	res := evalTestScript(t, `(split-whitespace " alpha\tbeta  gamma\n")`)
	v, ok := res.(*ArrayVector)
	if !ok {
		t.Fatalf("expected vector, got %T", res)
	}
	if v.Count() != 3 || v.At(0).(String).S != "alpha" || v.At(2).(String).S != "gamma" {
		t.Fatalf("unexpected split result: %s", v.ToString(false))
	}
}
