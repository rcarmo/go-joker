package core

import "testing"

func TestIRTypedEvalGate(t *testing.T) {
	t.Setenv("JOKER_IR_TYPED", "1")
	requireInt(t, evalTestScript(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4)
      (count s)
      (recur (inc i) (str s (nth dna i))))))`), 4)
}
