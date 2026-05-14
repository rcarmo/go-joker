package core

import "testing"

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
