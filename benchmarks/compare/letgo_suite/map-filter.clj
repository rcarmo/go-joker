;; Mirrored from nooga/let-go benchmark/map-filter.clj
(reduce + 0
  (take 100
    (filter even?
      (map #(* % %) (range 10000)))))
