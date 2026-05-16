;; Mirrored from nooga/let-go benchmark/loop-recur.clj.
;; Uses equality at the loop bound so go-joker's fixed-width integer loop path
;; cannot overflow before the branch exits.
(loop [i 0 acc 0]
  (if (= i 1000000)
    acc
    (recur (inc i) (+ acc i))))
