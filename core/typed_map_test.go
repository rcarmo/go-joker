package core

import "testing"

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
