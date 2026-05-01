package core

import "testing"

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
