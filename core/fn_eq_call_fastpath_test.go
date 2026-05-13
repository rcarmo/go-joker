package core

import "testing"

func TestIREqSupportsStringsAndChars(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [f (fn [c]
                                  (if (= c "A") 1
                                  (if (= c "T") 2 3)))]
  (loop [i 0 acc 0]
    (if (= i 3)
      acc
      (recur (inc i) (+ acc (f (str (nth "ATA" i))))))))`), 4)
}
