package clbgscripts

// --- fannkuch-redux (N=7) ---

var FannkuchScript = `(let [n 7
      init-perm (loop [i 0 v []] (if (= i n) v (recur (+ i 1) (conj v i))))
      flip (fn [perm]
        (loop [lo 0 hi (nth perm 0) p perm]
          (if (< lo hi)
            (let [tmp (nth p lo)]
              (recur (+ lo 1) (- hi 1) (assoc (assoc p lo (nth p hi)) hi tmp)))
            p)))
      count-flips (fn [perm]
        (loop [p perm flips 0]
          (if (= (nth p 0) 0)
            flips
            (recur (flip p) (+ flips 1)))))
      init-count (loop [i 0 v []] (if (= i n) v (recur (+ i 1) (conj v 0))))
      rotate-left (fn [perm r]
        (let [p0 (nth perm 0)]
          (loop [i 0 p perm]
            (if (= i r)
              (assoc p r p0)
              (recur (+ i 1) (assoc p i (nth p (+ i 1))))))))]
  (loop [perm1 init-perm count init-count max-flips 0 checksum 0 r n sign 1]
    (let [prepared (loop [rr r c count]
                     (if (not= rr 1)
                       (recur (- rr 1) (assoc c (- rr 1) rr))
                       [rr c]))
          r1 (nth prepared 0)
          count1 (nth prepared 1)
          flips (count-flips perm1)
          mf (if (< max-flips flips) flips max-flips)
          cs (+ checksum (* sign flips))
          next-state (loop [rr r1 p perm1 c count1]
                       (if (= rr n)
                         [:done p c rr sign]
                         (let [p2 (rotate-left p rr)
                               c2 (assoc c rr (- (nth c rr) 1))]
                           (if (< 0 (nth c2 rr))
                             [:next p2 c2 rr (* sign -1)]
                             (recur (+ rr 1) p2 c2)))))]
      (if (= (nth next-state 0) :done)
        (+ (* mf 1000) cs)
        (recur (nth next-state 1) (nth next-state 2) mf cs (nth next-state 3) (nth next-state 4))))))`

// --- mandelbrot (N=200, max-iter=50) ---

var MandelbrotScript = `(letfn [(pixel [cr ci]
    (loop [zr 0.0 zi 0.0 i 0]
      (if (= i 50)
        1
        (let [zr2 (* zr zr)
              zi2 (* zi zi)]
          (if (< 4.0 (+ zr2 zi2))
            0
            (recur (+ (- zr2 zi2) cr)
                   (+ (* 2.0 (* zr zi)) ci)
                   (+ i 1)))))))]
  (loop [y 0 count 0]
    (if (= y 40)
      count
      (let [rc (loop [x 0 rc 0]
                 (if (= x 40)
                   rc
                   (recur (+ x 1) (+ rc
                     (pixel (- (/ (* 2.0 x) 40) 1.5)
                            (- (/ (* 2.0 y) 40) 1.0))))))]
        (recur (+ y 1) (+ count rc))))))`

// --- fasta (N=1000) — sequence generation ---

var FastaScript = `(let [im 139968
      ia 3877
      ic 29573
      alu "GGCCGGGCGCGGTGGCTCACGCCTGTAATCCCAGCACTTTGGGAGGCCGAGGCGGGCGGATCACCTGAGGTCAGGAGTTCGAGACCAGCCTGGCCAACATGGTGAAACCCCGTCTCTACTAAAAATACAAAAATTAGCCGGGCGTGGTGGCGCGCGCCTGTAATCCCAGCTACTCGGGAGGCTGAGGCAGGAGAATCGCTTGAACCCGGGAGGCGGAGGTTGCAGTGAGCCGAGATCGCGCCACTGCACTCCAGCCTGGGCGACAGAGCGAGACTCCGTCTCAAA"]
  (loop [i 0 seed 42 checksum 0]
    (if (= i 1000)
      (+ checksum seed)
      (let [new-seed (rem (+ (* seed ia) ic) im)
            idx (rem new-seed (count alu))]
        (recur (+ i 1) new-seed (+ checksum idx))))))`

// --- pidigits (N=100) ---
var PidigitsScript = `(loop [i 0
       q 1 r 0 t 1 k 1 n 3 l 3
       digits 0 checksum 0]
  (if (= digits 27)
    checksum
    (if (< (- (+ (* 4 q) r) t) (* n t))
      (recur (+ i 1) (* q 10) (* 10 (- r (* n t))) t k
             (- (/ (* 10 (+ (* 3 q) r)) t) (* 10 n)) l
             (+ digits 1) (+ checksum n))
      (let [q2 (* q k)
            r2 (* (+ (* 2 q) r) l)
            t2 (* t l)
            k2 (+ k 1)
            n2 (/ (+ (* q (+ (* 7 k) 2)) (* r l)) t2)
            l2 (+ l 2)]
        (recur i q2 r2 t2 k2 n2 l2 digits checksum)))))`

// --- k-nucleotide (simplified) ---
var KNucleotideScript = `(let [dna "GGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT"
      len-dna (count dna)]
  (loop [frame 1 total 0]
    (if (= frame 4)
      total
      (let [freq (loop [i 0 m {}]
                   (if (< (- len-dna frame) i)
                     m
                     (let [k (loop [j 0 s ""]
                               (if (= j frame)
                                 s
                                 (recur (+ j 1) (str s (nth dna (+ i j))))))]
                       (recur (+ i 1) (assoc m k (+ 1 (get m k 0)))))))]
        (recur (+ frame 1) (+ total (count freq)))))))`

// --- reverse-complement (simplified) ---
var ReverseComplementScript = `(let [dna "GGCCGGGCGCGGTGGCTCACGCCTGTAATCCCAGCACTTTGGGAGGCCGAGGCGGGCGGATCACCTGAGGTCAGGAGTTCGAGACCAGCCTGGCCAACATGGTGAAACCCCGTCTCTACTAAAAATACAAAAATTAGCCGGGCGTGGTGGCGCGCGCCTGTAATCCCAGCTACTCGGGAGGCTGAGGCAGGAGAAT"
      len-dna (count dna)]
  (loop [i 0 result ""]
    (if (= i len-dna)
      (count result)
      (let [c (nth dna (- (- len-dna 1) i))
            comp-c (if (= c \G) "C"
                   (if (= c \C) "G"
                   (if (= c \A) "T"
                   (if (= c \T) "A" (str c)))))]
        (recur (+ i 1) (str result comp-c))))))`

// --- regex-redux (simplified) ---
var RegexReduxScript = `(let [input "agggtaaa|tttaccct ggtattttaatttatagt aactatagtattttaatttatagtagtattttaatttatagt cattttaatttatagtaactatagtattttaatttatagt agggtaaa tttaccct agggtaaatttaccct agggtaaa|tttaccct"
      patterns ["agggtaaa|tttaccct"
                "[cgt]gggtaaa|tttaccc[acg]"
                "a[act]ggtaaa|tttacc[agt]t"
                "ag[act]gtaaa|tttac[agt]ct"
                "agg[act]taaa|ttta[agt]cct"
                "aggg[acg]aaa|ttt[cgt]ccct"
                "agggt[cgt]aa|tt[acg]accct"
                "agggta[cgt]a|t[acg]taccct"
                "agggtaa[cgt]|[acg]ttaccct"]]
  (loop [i 0 total 0]
    (if (= i (count patterns))
      total
      (let [pat (nth patterns i)
            matches (re-seq (re-pattern pat) input)
            c (count matches)]
        (recur (+ i 1) (+ total c))))))`

var KNucleotideBestJokerScript = `(bench-kmer-distinct-total "GGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT TATTTTAATTTATAGTATTTTAATTTATAGT" 3)`

var ReverseComplementBestJokerScript = `(bench-reverse-complement-count "GGCCGGGCGCGGTGGCTCACGCCTGTAATCCCAGCACTTTGGGAGGCCGAGGCGGGCGGATCACCTGAGGTCAGGAGTTCGAGACCAGCCTGGCCAACATGGTGAAACCCCGTCTCTACTAAAAATACAAAAATTAGCCGGGCGTGGTGGCGCGCGCCTGTAATCCCAGCTACTCGGGAGGCTGAGGCAGGAGAAT")`

var RegexReduxBestJokerScript = `(bench-regex-count "agggtaaa|tttaccct ggtattttaatttatagt aactatagtattttaatttatagtagtattttaatttatagt cattttaatttatagtaactatagtattttaatttatagt agggtaaa tttaccct agggtaaatttaccct agggtaaa|tttaccct"
  ["agggtaaa|tttaccct"
   "[cgt]gggtaaa|tttaccc[acg]"
   "a[act]ggtaaa|tttacc[agt]t"
   "ag[act]gtaaa|tttac[agt]ct"
   "agg[act]taaa|ttta[agt]cct"
   "aggg[acg]aaa|ttt[cgt]ccct"
   "agggt[cgt]aa|tt[acg]accct"
   "agggta[cgt]a|t[acg]taccct"
   "agggtaa[cgt]|[acg]ttaccct"])`
