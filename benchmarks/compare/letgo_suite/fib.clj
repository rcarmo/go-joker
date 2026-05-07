;; Mirrored from nooga/let-go benchmark/fib.clj
(defn fib [n]
  (if (<= n 1)
    n
    (+ (fib (- n 1)) (fib (- n 2)))))

(fib 35)
