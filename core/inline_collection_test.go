package core

import "testing"

func TestIRInlineCollectionHelper(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [pick (fn [v i] (+ (nth v i) 1))
                                      xs [1 2 3 4]]
  (loop [i 0 acc 0]
    (if (= i 4)
      acc
      (recur (inc i) (+ acc (pick xs i))))))`), 14)
}
