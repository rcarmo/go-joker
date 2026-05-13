package core

import "testing"

func TestIRLetCaptureSlotCollisionRegression(t *testing.T) {
	// The inner let value captures outer ks, which used to allocate the same
	// IR slot as the let binding k. That collision disabled IR compilation for
	// map-update style loops and kept them on the slower tree-walker path.
	expr := compileTestExpr(t, `(let [ks [:k0 :k1 :k2 :k3]]
  (loop [i 0 m {}]
    (if (= i 20)
      (+ (get m :k0 0) (get m :k1 0))
      (let [k (nth ks (rem i 4))]
        (recur (inc i) (assoc m k (+ 1 (get m k 0))))))))`)
	let := expr.(*LetExpr)
	loop := let.body[0].(*LoopExpr)
	if prog := irCompile(loop); prog == nil {
		t.Fatal("expected IR compilation for captured inner let")
	}
	requireInt(t, Eval(expr, nil), 10)
}

func TestIRNestedLoopCaptureSlotCollisionRegression(t *testing.T) {
	expr := compileTestExpr(t, `(let [xs [1 2 3]]
  (loop [i 0 acc 0]
    (if (= i 3)
      acc
      (let [x (nth xs i)]
        (recur (inc i)
               (+ acc (loop [j 0 sum 0]
                        (if (= j x)
                          sum
                          (recur (inc j) (+ sum x))))))))))`)
	let := expr.(*LetExpr)
	loop := let.body[0].(*LoopExpr)
	if prog := irCompile(loop); prog == nil {
		t.Fatal("expected IR compilation for nested loop with captured init")
	}
	requireInt(t, Eval(expr, nil), 14)
}
