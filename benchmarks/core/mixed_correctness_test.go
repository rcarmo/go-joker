package core_test

import (
	. "github.com/rcarmo/go-joker/core"
	"testing"
)

func TestMixedBenchmarkScriptResults(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   int
	}{
		{"inline-small-helper", `(let [f (fn [x] (+ x 1))]
  (loop [i 0 acc 0]
    (if (= i 1000)
      acc
      (recur (inc i) (+ acc (f i))))))`, 500500},
		{"transient-vector-loop", `(loop [i 0 v []]
  (if (= i 100)
    (count v)
    (recur (inc i) (conj v i))))`, 100},
		{"typed-string-loop", `(let [dna "GGTATTTTAATTTATAGT"]
  (loop [i 0 s ""]
    (if (= i 128)
      (count s)
      (recur (inc i) (str s (nth dna (rem i 18)))))))`, 128},
		{"typed-string-int-map-loop", `(let [ks ["aa" "bb" "cc" "dd"]]
  (loop [i 0 m {}]
    (if (= i 1000)
      (+ (get m "aa" 0) (get m "dd" 0))
      (let [k (nth ks (rem i 4))]
        (recur (inc i) (assoc m k (inc (get m k 0))))))))`, 500},
		{"typed-int-vector-loop", `(loop [i 0 v []]
  (if (= i 1000)
    (+ (nth v 0) (nth v 999))
    (recur (inc i) (conj v i))))`, 999},
		{"inline-collection-helper-loop", `(let [pick (fn [v i] (+ (nth v i) 1))
                                  xs [1 2 3 4 5 6 7 8]]
  (loop [i 0 acc 0]
    (if (= i 1000)
      acc
      (recur (inc i) (+ acc (pick xs (rem i 8)))))))`, 5500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Eval(compileBenchExpr(t, tt.script), nil)
			if got == nil || !got.Equals(MakeInt(tt.want)) {
				t.Fatalf("%s = %v, want %d", tt.name, got, tt.want)
			}
		})
	}
}
