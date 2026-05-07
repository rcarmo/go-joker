;; Mirrored from nooga/let-go benchmark/persistent-map.clj
(reduce (fn [m i] (assoc m i (* i i)))
        {}
        (range 10000))
