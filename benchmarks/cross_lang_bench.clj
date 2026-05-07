;; cross_lang_bench.clj — let-go (Clojure-on-Go) port of cross_lang_bench.py
;; Run with: letgo benchmarks/cross_lang_bench.clj
;; https://github.com/nooga/let-go
(require 'math)

(defn now-ns [] (System/nanoTime))

(defn bench [name f]
  (let [iters 5]
    (loop [i 0 sum 0 result nil]
      (if (= i iters)
        (let [avg-ms (/ (double sum) iters 1000000.0)]
          (println (format "%-30s %10.2f ms/op  (result: %s)" name avg-ms (str result))))
        (let [t0 (now-ns)
              r  (f)
              dt (- (now-ns) t0)]
          (recur (inc i) (+ sum dt) r))))))

;; --- Arithmetic loop ---
(defn arithmetic-loop []
  (loop [i 0 s 0]
    (if (< i 100000)
      (recur (inc i) (+ s (mod (* i 7) 11)))
      s)))

;; --- Recursive fib ---
(defn fib [n]
  (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2)))))

(defn recursive-fib []
  (loop [i 0 s 0]
    (if (< i 3)
      (recur (inc i) (+ s (fib 24)))
      s)))

;; --- Tail-recursive sum ---
(defn tail-recursive-sum []
  (loop [n 100000 acc 0]
    (if (> n 0) (recur (dec n) (+ acc n)) acc)))

;; --- N-body (100 steps, 5 bodies) ---
(defn round6 [x]
  (let [m 1000000.0]
    (/ (math/round (* x m)) m)))

(defn nbody []
  (let [pi 3.141592653589793
        solar-mass (* 4.0 pi pi)
        dpy 365.24
        bodies (double-array
                 [;; Sun
                  0.0 0.0 0.0 0.0 0.0 0.0 solar-mass
                  ;; Jupiter
                  4.84143144246472090 -1.16032004402742839 -0.103622044471123109
                  (* 0.00166007664274403694 dpy)
                  (* 0.00769901118419740425 dpy)
                  (* -0.0000690460016972063023 dpy)
                  (* 0.000954791938424326609 solar-mass)
                  ;; Saturn
                  8.34336671824457987 4.12479856412430479 -0.403523417114321381
                  (* -0.00276742510726862411 dpy)
                  (* 0.00499852801234917238 dpy)
                  (* 0.0000230417297573763929 dpy)
                  (* 0.000285885980666130812 solar-mass)
                  ;; Uranus
                  12.8943695621391310 -15.1111514016986312 -0.223307578892655734
                  (* 0.00296460137564761618 dpy)
                  (* 0.00237847173959480950 dpy)
                  (* -0.0000296589568540237556 dpy)
                  (* 0.0000436624404335156298 solar-mass)
                  ;; Neptune
                  15.3796971148509165 -25.9193146099879641 0.179258772950371181
                  (* 0.00268067772490389322 dpy)
                  (* 0.00162824170038242295 dpy)
                  (* -0.0000951592254519715870 dpy)
                  (* 0.0000515138902046611451 solar-mass)])
        dt 0.01
        n 5]
    (dotimes [_ 100]
      (dotimes [i n]
        (let [io (* i 7)]
          (loop [j (inc i)]
            (when (< j n)
              (let [jo (* j 7)
                    dx (- (aget bodies (+ io 0)) (aget bodies (+ jo 0)))
                    dy (- (aget bodies (+ io 1)) (aget bodies (+ jo 1)))
                    dz (- (aget bodies (+ io 2)) (aget bodies (+ jo 2)))
                    d2 (+ (* dx dx) (* dy dy) (* dz dz))
                    d  (math/sqrt d2)
                    mag (/ dt (* d2 d))
                    mi (aget bodies (+ io 6))
                    mj (aget bodies (+ jo 6))]
                (aset bodies (+ io 3) (- (aget bodies (+ io 3)) (* dx mj mag)))
                (aset bodies (+ io 4) (- (aget bodies (+ io 4)) (* dy mj mag)))
                (aset bodies (+ io 5) (- (aget bodies (+ io 5)) (* dz mj mag)))
                (aset bodies (+ jo 3) (+ (aget bodies (+ jo 3)) (* dx mi mag)))
                (aset bodies (+ jo 4) (+ (aget bodies (+ jo 4)) (* dy mi mag)))
                (aset bodies (+ jo 5) (+ (aget bodies (+ jo 5)) (* dz mi mag)))
                (recur (inc j)))))))
      (dotimes [i n]
        (let [io (* i 7)]
          (aset bodies (+ io 0) (+ (aget bodies (+ io 0)) (* dt (aget bodies (+ io 3)))))
          (aset bodies (+ io 1) (+ (aget bodies (+ io 1)) (* dt (aget bodies (+ io 4)))))
          (aset bodies (+ io 2) (+ (aget bodies (+ io 2)) (* dt (aget bodies (+ io 5))))))))
    (let [e (loop [i 0 e 0.0]
              (if (< i n)
                (let [io (* i 7)
                      mi (aget bodies (+ io 6))
                      vx (aget bodies (+ io 3))
                      vy (aget bodies (+ io 4))
                      vz (aget bodies (+ io 5))
                      e1 (+ e (* 0.5 mi (+ (* vx vx) (* vy vy) (* vz vz))))
                      e2 (loop [j (inc i) e e1]
                           (if (< j n)
                             (let [jo (* j 7)
                                   dx (- (aget bodies (+ io 0)) (aget bodies (+ jo 0)))
                                   dy (- (aget bodies (+ io 1)) (aget bodies (+ jo 1)))
                                   dz (- (aget bodies (+ io 2)) (aget bodies (+ jo 2)))
                                   d  (math/sqrt (+ (* dx dx) (* dy dy) (* dz dz)))]
                               (recur (inc j) (- e (/ (* mi (aget bodies (+ jo 6))) d))))
                             e))]
                  (recur (inc i) e2))
                e))]
      (round6 e))))

;; --- Spectral norm (N=50) ---
(defn spectral-norm []
  (let [n 50
        a-fn (fn [i j] (/ 1.0 (+ (quot (* (+ i j) (+ i j 1)) 2) i 1)))
        mul-Av (fn [v]
                 (let [out (double-array n)]
                   (dotimes [i n]
                     (let [s (loop [j 0 s 0.0]
                               (if (< j n)
                                 (recur (inc j) (+ s (* (a-fn i j) (aget v j))))
                                 s))]
                       (aset out i s)))
                   out))
        mul-Atv (fn [v]
                  (let [out (double-array n)]
                    (dotimes [i n]
                      (let [s (loop [j 0 s 0.0]
                                (if (< j n)
                                  (recur (inc j) (+ s (* (a-fn j i) (aget v j))))
                                  s))]
                        (aset out i s)))
                    out))
        mul-AtAv (fn [v] (mul-Atv (mul-Av v)))]
    (loop [k 0 u (double-array n 1.0) v (double-array n)]
      (if (< k 10)
        (let [v2 (mul-AtAv u)]
          (recur (inc k) (mul-AtAv v2) v2))
        (let [vBv (loop [i 0 s 0.0]
                    (if (< i n) (recur (inc i) (+ s (* (aget u i) (aget v i)))) s))
              vv  (loop [i 0 s 0.0]
                    (if (< i n) (recur (inc i) (+ s (* (aget v i) (aget v i)))) s))]
          (round6 (math/sqrt (/ vBv vv))))))))

;; --- Binary trees (depth 14) ---
(defn make-tree [d]
  (if (= d 0) nil [(make-tree (- d 1)) (make-tree (- d 1))]))

(defn check-tree [t]
  (if (nil? t) 1 (+ 1 (check-tree (nth t 0)) (check-tree (nth t 1)))))

(defn binary-trees []
  (loop [d 4 total 0]
    (if (< d 15)
      (let [iters (bit-shift-left 1 (- 14 d))
            c (loop [k 0 s 0]
                (if (< k iters)
                  (recur (inc k) (+ s (check-tree (make-tree d))))
                  s))]
        (recur (inc d) (+ total c)))
      total)))

;; --- Map update loop (matches BenchmarkEvalMapUpdateLoop shape) ---
(defn map-update-loop []
  (let [ks [:k0 :k1 :k2 :k3 :k4 :k5 :k6 :k7 :k8 :k9 :k10 :k11 :k12 :k13 :k14 :k15]]
    (loop [i 0 m {}]
      (if (= i 5000)
        (+ (get m :k0 0) (+ (get m :k7 0) (get m :k15 0)))
        (let [k (nth ks (mod i 16))]
          (recur (inc i) (assoc m k (inc (get m k 0)))))))))

;; --- Word frequency (matches BenchmarkEvalWordFrequency shape) ---
(defn word-frequency []
  (let [text (apply str (interpose " "
                    (loop [i 0 out []]
                      (if (< i 4000)
                        (let [w (nth ["alpha" "beta" "gamma" "delta" "epsilon" "zeta" "eta" "theta"] (mod i 8))]
                          (recur (inc i) (conj out w)))
                        out))))
        words (re-seq #"\S+" text)]
    (loop [i 0 counts {}]
      (if (= i (count words))
        (+ (get counts "theta" 0) (get counts "alpha" 0))
        (let [w (nth words i)]
          (recur (inc i) (assoc counts w (inc (get counts w 0)))))))))

;; --- Fannkuch-redux (N=7) ---
(defn fannkuch []
  (let [n 7
        perm (int-array (range n))
        c    (int-array n)]
    (loop [max-flips 0 checksum 0 sign 1]
      (let [;; count flips on a copy
            p (let [a (int-array n)]
                (dotimes [k n] (aset a k (aget perm k)))
                a)
            flips (loop [f 0]
                    (if (= (aget p 0) 0)
                      f
                      (let [k (aget p 0)
                            ;; reverse p[0..k] inclusive
                            _ (loop [a 0 b k]
                                (when (< a b)
                                  (let [tmp (aget p a)]
                                    (aset p a (aget p b))
                                    (aset p b tmp)
                                    (recur (inc a) (dec b)))))]
                        (recur (inc f)))))
            mf (if (> flips max-flips) flips max-flips)
            cs (+ checksum (if (= sign 1) flips (- flips)))
            ;; next permutation (Heap's algorithm)
            done? (loop [i 1]
                    (if (>= i n)
                      true
                      (do
                        (aset c i (inc (aget c i)))
                        (if (< (aget c i) (inc i))
                          (do
                            (if (= 0 (mod (inc i) 2))
                              (let [t (aget perm 0)]
                                (aset perm 0 (aget perm i))
                                (aset perm i t))
                              (let [t (aget perm 0)]
                                (aset perm 0 (aget perm 1))
                                (aset perm 1 t)))
                            false)
                          (do (aset c i 0) (recur (inc i)))))))]
        (if done?
          (+ (* mf 1000) cs)
          (recur mf cs (- sign)))))))

;; --- Mandelbrot (N=40, max-iter=50) ---
(defn mandelbrot []
  (let [n 40 limit-sq 4.0 max-iter 50]
    (loop [y 0 count 0]
      (if (< y n)
        (let [row (loop [x 0 c 0]
                    (if (< x n)
                      (let [cr (- (/ (* 2.0 x) n) 1.5)
                            ci (- (/ (* 2.0 y) n) 1.0)
                            inside (loop [zr 0.0 zi 0.0 k 0]
                                     (if (>= k max-iter)
                                       1
                                       (let [zr2 (* zr zr)
                                             zi2 (* zi zi)]
                                         (if (> (+ zr2 zi2) limit-sq)
                                           0
                                           (recur (+ (- zr2 zi2) cr)
                                                  (+ (* 2.0 zr zi) ci)
                                                  (inc k))))))]
                        (recur (inc x) (+ c inside)))
                      c))]
          (recur (inc y) (+ count row)))
        count))))

;; --- Fasta (N=1000) ---
(defn fasta []
  (let [im 139968 ia 3877 ic 29573
        alu "GGCCGGGCGCGGTGGCTCACGCCTGTAATCCCAGCACTTTGGGAGGCCGAGGCGGGCGGATCACCTGAGGTCAGGAGTTCGAGACCAGCCTGGCCAACATGGTGAAACCCCGTCTCTACTAAAAATACAAAAATTAGCCGGGCGTGGTGGCGCGCGCCTGTAATCCCAGCTACTCGGGAGGCTGAGGCAGGAGAATCGCTTGAACCCGGGAGGCGGAGGTTGCAGTGAGCCGAGATCGCGCCACTGCACTCCAGCCTGGGCGACAGAGCGAGACTCCGTCTCAAA"
        L (count alu)]
    (loop [i 0 seed 42 checksum 0]
      (if (< i 1000)
        (let [s (mod (+ (* seed ia) ic) im)]
          (recur (inc i) s (+ checksum (mod s L))))
        (+ checksum seed)))))

;; --- K-nucleotide (simplified) ---
(defn knucleotide []
  (let [dna "GGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT"
        L (count dna)]
    (loop [frame 1 total 0]
      (if (< frame 4)
        (let [n (- L frame -1)
              freq (loop [i 0 m (transient {})]
                     (if (< i n)
                       (let [k (subs dna i (+ i frame))]
                         (recur (inc i) (assoc! m k (inc (get m k 0)))))
                       (persistent! m)))]
          (recur (inc frame) (+ total (count freq))))
        total))))

;; --- Reverse-complement (simplified) ---
(defn reverse-complement []
  (let [dna "GGCCGGGCGCGGTGGCTCACGCCTGTAATCCCAGCACTTTGGGAGGCCGAGGCGGGCGGATCACCTGAGGTCAGGAGTTCGAGACCAGCCTGGCCAACATGGTGAAACCCCGTCTCTACTAAAAATACAAAAATTAGCCGGGCGTGGTGGCGCGCGCCTGTAATCCCAGCTACTCGGGAGGCTGAGGCAGGAGAAT"
        comp {\A \T \T \A \G \C \C \G \space \space}]
    (count (apply str (map #(get comp % %) (reverse dna))))))

;; --- Regex-redux (simplified) ---
(defn regex-redux []
  (let [inp "agggtaaa|tttaccct ggtattttaatttatagt aactatagtattttaatttatagt cattttaatttatagtaactatagtattttaatttatagt agggtaaa tttaccct agggtaaatttaccct agggtaaa|tttaccct"
        pats [#"agggtaaa|tttaccct"
              #"[cgt]gggtaaa|tttaccc[acg]"
              #"a[act]ggtaaa|tttacc[agt]t"
              #"ag[act]gtaaa|tttac[agt]ct"
              #"agg[act]taaa|ttta[agt]cct"
              #"aggg[acg]aaa|ttt[cgt]ccct"
              #"agggt[cgt]aa|tt[acg]accct"
              #"agggta[cgt]a|t[acg]taccct"
              #"agggtaa[cgt]|[acg]ttaccct"]]
    (reduce + (map #(count (re-seq % inp)) pats))))

;; --- Pidigits (N=27) ---
;; Uses arbitrary-precision integers (BigInt suffix N) — int64 overflows here.
(defn pidigits []
  (loop [q 1N r 0N t 1N k 1N n 3N l 3N digits 0 checksum 0]
    (if (>= digits 27)
      checksum
      (if (< (- (+ (* 4 q) r) t) (* n t))
        (recur (* q 10) (* 10 (- r (* n t))) t k
               (- (quot (* 10 (+ (* 3 q) r)) t) (* 10 n)) l
               (inc digits) (+ checksum n))
        (let [q2 (* q k)
              r2 (* (+ (* 2 q) r) l)
              t2 (* t l)
              k2 (inc k)
              n2 (quot (+ (* q (+ (* 7 k) 2)) (* r l)) t2)
              l2 (+ l 2)]
          (recur q2 r2 t2 k2 n2 l2 digits checksum))))))

(println "let-go benchmarks (5 iterations each)")
(println (apply str (repeat 60 "=")))
(bench "arithmetic_loop"     arithmetic-loop)
(bench "recursive_fib"       recursive-fib)
(bench "tail_recursive_sum"  tail-recursive-sum)
(bench "nbody_100steps"      nbody)
(bench "spectral_norm_50"    spectral-norm)
(bench "binary_trees_14"     binary-trees)
(bench "fannkuch_7"          fannkuch)
(bench "mandelbrot_200"      mandelbrot)
(bench "fasta_1000"          fasta)
(bench "knucleotide"         knucleotide)
(bench "reverse_complement"  reverse-complement)
(bench "map_update_loop"     map-update-loop)
(bench "word_frequency"      word-frequency)
(bench "regex_redux"         regex-redux)
(bench "pidigits_27"         pidigits)
