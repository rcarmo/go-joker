package core

import "testing"

func TestIRInlineSmallHelper(t *testing.T) {
	t.Setenv("JOKER_IR_INLINE", "1")
	requireInt(t, evalTestScript(t, `(let [f (fn [x] (+ x 1))]
  (loop [i 0 acc 0]
    (if (= i 4)
      acc
      (recur (inc i) (+ acc (f i))))))`), 10)
}
