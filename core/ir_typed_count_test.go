package core

import "testing"

func TestIRTypedCountObjectVector(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [xs ["a" "b" "c"]]
  (loop [i 0 s ""]
    (if (= i (count xs))
      (count s)
      (recur (inc i) (str s "x")))))`), 3)
}
