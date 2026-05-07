;; Mirrored from nooga/let-go benchmark/transducers.clj
(transduce
  (comp (map #(* % %))
        (filter even?)
        (take 100))
  + 0
  (range 10000))
