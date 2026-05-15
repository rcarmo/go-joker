package core

import "testing"

func TestIRInlineSlotCollision(t *testing.T) {
	// Regression test: inlined helper parameter must not collide with
	// capture slots or loop bindings when the fn's parameter frame
	// matches the caller's loop frame.
	t.Setenv("JOKER_IR_INLINE", "force")

	// sq's x param was at {frame:1, index:0} — same as loop var i
	requireInt(t, evalTestScript(t, `(let [sq (fn [x] (* x x))
	              v [10 20 30]]
	  (loop [i 0 s 0]
	    (if (= i 3) s
	      (recur (inc i) (+ s (sq (nth v i)))))))`), 1400)

	// Two-arg helper with both params colliding
	requireInt(t, evalTestScript(t, `(let [f (fn [a b] (+ (* a a) b))
	              v [1 2 3 4 5]]
	  (loop [i 0 s 0]
	    (if (= i 5) s
	      (recur (inc i) (+ s (f (nth v i) i))))))`),
		// f(1,0)+f(2,1)+f(3,2)+f(4,3)+f(5,4) = 1+5+11+19+29 = 65
		65)
}

func TestIRInlineSmallHelper(t *testing.T) {
	t.Setenv("JOKER_IR_INLINE", "1")
	requireInt(t, evalTestScript(t, `(let [f (fn [x] (+ x 1))]
  (loop [i 0 acc 0]
    (if (= i 4)
      acc
      (recur (inc i) (+ acc (f i))))))`), 10)
}

func TestIRInlineCollectionHelper(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [pick (fn [v i] (+ (nth v i) 1))
                                      xs [1 2 3 4]]
  (loop [i 0 acc 0]
    (if (= i 4)
      acc
      (recur (inc i) (+ acc (pick xs i))))))`), 14)
}
