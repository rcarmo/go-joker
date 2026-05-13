package core

import "testing"

func TestIRTypedMapRejectsNonStringKeysAndFallsBack(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [ks [:k0 :k1]]
  (loop [i 0 m {}]
    (if (= i 4)
      (+ (get m :k0 0) (get m :k1 0))
      (let [k (nth ks (rem i 2))]
        (recur (inc i) (assoc m k (inc (get m k 0))))))))`), 4)
}
