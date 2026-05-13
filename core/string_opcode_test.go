package core

import "testing"

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
