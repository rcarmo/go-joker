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

func TestIRTypedCountObjectVector(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [xs ["a" "b" "c"]]
  (loop [i 0 s ""]
    (if (= i (count xs))
      (count s)
      (recur (inc i) (str s "x")))))`), 3)
}

func TestIRTypedEvalGate(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED", "1")
	requireInt(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      (count s)
      (recur (inc i) (str s (nth dna i))))))`), 4)
}

func TestIRTypedMapRejectsNonStringKeysAndFallsBack(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [ks [:k0 :k1]]
  (loop [i 0 m {}]
    (if (= i 4)
      (+ (get m :k0 0) (get m :k1 0))
      (let [k (nth ks (rem i 2))]
        (recur (inc i) (assoc m k (inc (get m k 0))))))))`), 4)
}

func TestIRTypedNestedStringLoop(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 total 0]
    (if (= i 3)
      total
      (let [k (loop [j 0 s ""]
                (if (= j 2)
                  s
                  (recur (inc j) (str s (nth dna (+ i j))))))]
        (recur (inc i) (+ total (count k)))))))`), 6)
}

func TestIRTypedIntVector(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED_VEC", "1")
	requireInt(t, evalTestScript(t, `(loop [i 0 v []]
  (if (= i 5)
    (+ (nth v 0) (nth v 4))
    (recur (inc i) (conj v i))))`), 4)
}

func TestIRTypedGenericStringNth(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 s "ACGT" acc ""]
  (if (= i 4)
    acc
    (recur (inc i) s (str acc (nth s i)))))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("expected IR")
	}
	got := irExecTyped(prog, []Object{Int{I: 0}, String{S: "ACGT"}, String{S: ""}})
	requireString(t, got, "ACGT")
}

func TestIRTypedStringIntMap(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED_MAP", "1")
	requireInt(t, evalTestScript(t, `(loop [i 0 m {}]
  (if (= i 4)
    (get m "aa" 0)
    (recur (inc i) (assoc m "aa" (inc (get m "aa" 0))))))`), 4)
	requireBool(t, evalTestScript(t, `(nil? (loop [i 0 m {}]
  (if (= i 1)
    (get m "missing")
    (recur (inc i) m))))`), true)
}

func TestIRTypedVectorNthForStringMap(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED_MAP", "1")
	requireInt(t, evalTestScript(t, `(let [ks ["aa" "bb"]]
  (loop [i 0 m {}]
    (if (= i 4)
      (get m "aa" 0)
      (let [k (nth ks (rem i 2))]
        (recur (inc i) (assoc m k (inc (get m k 0))))))))`), 2)
}
