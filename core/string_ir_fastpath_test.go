package core

import "testing"

func TestStringNthFastUnicodeCorrectness(t *testing.T) {
	got := stringNthFast("abcdef", 3)
	if ch, ok := got.(Char); !ok || ch.Ch != 'd' {
		t.Fatalf("expected d, got %T %s", got, got.ToString(false))
	}
	got = stringNthFast("éclair", 1)
	if ch, ok := got.(Char); !ok || ch.Ch != 'c' {
		t.Fatalf("expected c, got %T %s", got, got.ToString(false))
	}
}

func TestIRNthStringFastPath(t *testing.T) {
	requireString(t, evalTestScript(t, `(loop [i 0 s ""]
  (if (= i 3)
    s
    (recur (inc i) (str s (nth "abcdef" i)))))`), "abc")
}

func TestCharToStringFast(t *testing.T) {
	if got := charToStringFast('A'); got != "A" {
		t.Fatalf("expected A, got %q", got)
	}
	if got := charToStringFast('é'); got != "é" {
		t.Fatalf("expected é, got %q", got)
	}
	if got := charToStringObjectFast('A'); got.(String).S != "A" {
		t.Fatalf("expected cached A object, got %T %s", got, got.ToString(false))
	}
	if got := charToStringObjectFast('é'); got.(String).S != "é" {
		t.Fatalf("expected unicode string object, got %T %s", got, got.ToString(false))
	}
}

func TestIRNthStringASCIIOpcode(t *testing.T) {
	expr := compileTestExpr(t, `(let [dna "ACGT"]
  (loop [i 0 acc ""]
    (if (= i 4)
      acc
      (recur (inc i) (str acc (nth dna i))))))`)
	letExpr := expr.(*LetExpr)
	loop := letExpr.body[0].(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Fatal("expected IR")
	}
	found := false
	for pc := 0; pc < len(prog.code); pc++ {
		if prog.code[pc] == irNthStringASCII {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected irNthStringASCII opcode")
	}
	requireString(t, Eval(expr, nil), "ACGT")
}
