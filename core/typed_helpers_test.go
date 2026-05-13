package core

import "testing"

func TestIRValueToString(t *testing.T) {
	cases := []struct {
		v irValue
		w string
	}{
		{irValue{tag: irValInt, i: 42}, "42"},
		{irMakeChar('A'), "A"},
		{irMakeString("abc", 3, true), "abc"},
		{irMakeBool(true), "true"},
		{irValue{tag: irValNil}, ""},
	}
	for _, c := range cases {
		if got := irValueToString(c.v); got != c.w {
			t.Fatalf("expected %q, got %q", c.w, got)
		}
	}
}

func TestIRTypedUnicodeCount(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 s "é"]
  (if (= i 2)
    (count s)
    (recur (inc i) (str s "é"))))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	requireInt(t, irExecTyped(prog, []Object{Int{I: 0}, String{S: "é"}}), 3)
}
