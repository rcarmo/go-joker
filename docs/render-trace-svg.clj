#!/usr/bin/env joker

(require '[joker.json :as json])
(require '[joker.string :as str])

(defn usage []
  (println "Usage: joker docs/render-trace-svg.clj INPUT.json OUTPUT.svg [title]")
  (System/exit 2))
(when (< (count *command-line-args*) 2) (usage))
(def input (nth *command-line-args* 0))
(def output (nth *command-line-args* 1))
(def explicit-title (when (>= (count *command-line-args*) 3) (nth *command-line-args* 2)))

(defn esc [s]
  (-> (str s) (str/replace "&" "&amp;") (str/replace "<" "&lt;") (str/replace ">" "&gt;") (str/replace "\"" "&quot;")))
(defn fmt-num [n]
  (cond (>= n 1000000000) (str (format "%.1f" (/ n 1000000000.0)) "G")
        (>= n 1000000) (str (format "%.1f" (/ n 1000000.0)) "M")
        (>= n 1000) (str (format "%.1f" (/ n 1000.0)) "K")
        :else (str n)))
(defn fmt-time [ns]
  (cond (>= ns 1000000000) (str (format "%.2f" (/ ns 1000000000.0)) "s")
        (>= ns 1000000) (str (format "%.1f" (/ ns 1000000.0)) "ms")
        (>= ns 1000) (str (format "%.1f" (/ ns 1000.0)) "µs")
        :else (str ns "ns")))
(defn sqrt [x] (joker.math/sqrt x))
(defn edge-value [e] (or (:nanos e) (:value e) (:count e) 0))
(defn edge-count [e] (or (:count e) (:samples e) 0))
(defn node-name [n] (or (:name n) (:symbol n) "?"))
(defn node-value [n] (or (:nanos n) (:value n) (:count n) 0))
(defn node-count [n] (or (:count n) (:samples n) 0))

(defn trace->graph [data]
  (cond
    (:nodes data)
    {:title (or explicit-title (:title data) "Go pprof trace")
     :subtitle "Sankey rendered by Joker from pprof nodes/links JSON"
     :nodes (:nodes data)
     :links (:links data)}

    (= (:type data) "go-joker-ir-profile")
    (let [ops (:ops data) counts (into {} (map (fn [o] [(:name o) o]) ops))]
      {:title (or explicit-title "Joker IR opcode trace")
       :subtitle (str "IR executions " (:execs data) " · timed opcode transitions")
       :nodes (map (fn [[name row]] {:name name :value (:nanos row) :count (:count row)}) counts)
       :links (map (fn [e] {:source (:source e) :target (:target e) :value (:nanos e) :count (:count e) :avg_nanos (:avg_nanos e)}) (:edges data))})

    (= (:type data) "go-joker-function-trace")
    (let [fns (:functions data) counts (into {} (map (fn [f] [(:name f) f]) fns))]
      {:title (or explicit-title "Joker function trace")
       :subtitle (str "Function calls " (:total data) " · timed call transitions")
       :nodes (map (fn [[name row]] {:name name :value (:nanos row) :count (:count row)}) counts)
       :links (map (fn [e] {:source (:source e) :target (:target e) :value (:nanos e) :count (:count e) :avg_nanos (:avg_nanos e)}) (:edges data))})

    (= (:type data) "go-joker-symbol-trace")
    (let [rows (concat (:resolves data) (:derefs data))]
      {:title (or explicit-title "Joker symbol trace")
       :subtitle (str "resolves " (:resolve_total data) " · derefs " (:deref_total data) " · count-only")
       :nodes (map (fn [r] {:name (:symbol r) :value (:count r) :count (:count r)}) rows)
       :links []})
    :else {:title (or explicit-title "Trace") :subtitle "Unknown trace" :nodes [] :links []}))

(defn used-node-names [links]
  (reduce (fn [s e] (conj (conj s (:source e)) (:target e))) #{} links))
(defn reachable? [adj start goal]
  (loop [frontier (list start) seen #{}]
    (cond
      (empty? frontier) false
      (= (first frontier) goal) true
      (contains? seen (first frontier)) (recur (rest frontier) seen)
      :else (recur (concat (rest frontier) (get adj (first frontier) [])) (conj seen (first frontier))))))

(defn depths [nodes links]
  ;; Assign left-to-right levels but ignore self/back edges so cycles and recursion do
  ;; not inflate depths over repeated relaxation passes.
  (let [used (if (seq links) (used-node-names links) (set (map node-name nodes)))
        adj (reduce (fn [m e] (update m (:source e) conj (:target e))) {} links)
        acyclic-links (filter (fn [e]
                                (let [s (:source e) t (:target e)]
                                  (and (not= s t) (not (reachable? adj t s))))) links)
        incoming (reduce (fn [m e] (update m (:target e) (fnil inc 0))) {} acyclic-links)
        roots (seq (filter #(zero? (get incoming % 0)) used))
        initial (merge (zipmap used (repeat 1)) (zipmap roots (repeat 0)))]
    (loop [i 0 d initial]
      (if (>= i 16) d
          (let [nd (reduce (fn [m e]
                             (let [s (:source e) t (:target e) want (inc (get m s 0))]
                               (if (> want (get m t 0)) (assoc m t want) m))) d acyclic-links)]
            (if (= nd d) d (recur (inc i) nd)))))))

(defn render-sankey [g]
  (let [links (take 80 (sort-by (fn [e] (- (edge-value e))) (:links g)))
        used (if (seq links) (used-node-names links) (set (map node-name (:nodes g))))
        nodes (filter #(contains? used (node-name %)) (:nodes g))
        raw-depth-map (depths nodes links)
        depth-ranks (zipmap (sort (set (vals raw-depth-map))) (range))
        depth-map (into {} (map (fn [[k v]] [k (get depth-ranks v 0)]) raw-depth-map))
        width 1400 height (max 700 (+ 120 (* 20 (count nodes))))
        top 70 bottom 30 left 30 right 260 node-w 6
        max-depth (max 1 (reduce max 0 (vals depth-map)))
        max-edge (max 1 (reduce max 1 (map edge-value links)))
        max-node (max 1 (reduce max 1 (map node-value nodes)))
        by-depth (group-by #(get depth-map (node-name %) 0) nodes)
        positioned (reduce-kv
                    (fn [m d arr]
                      (let [arr (sort-by (fn [n] (- (node-value n))) arr)
                            x (+ left (* (/ d max-depth) (- width left right)))
                            gap (/ (- height top bottom) (max 1 (count arr)))
                            h (min 28 (max 14 (* gap 0.42)))]
                        (loop [ys (+ top (/ gap 2)) xs arr acc m]
                          (if (empty? xs) acc
                              (recur (+ ys gap) (rest xs) (assoc acc (node-name (first xs)) {:x x :y ys :h h}))))))
                    {} by-depth)
        paths (apply str
                     (map-indexed
                      (fn [i e]
                        (let [a (get positioned (:source e)) b (get positioned (:target e))]
                          (if (and a b)
                            (let [sw (max 1 (* 18 (sqrt (/ (edge-value e) max-edge))))
                                  x1 (+ (:x a) node-w) x2 (:x b)
                                  c1 (+ x1 (max 40 (* (- x2 x1) 0.45)))
                                  c2 (- x2 (max 40 (* (- x2 x1) 0.45)))]
                              (str "<path d=\"M" x1 "," (:y a) " C" c1 "," (:y a) " " c2 "," (:y b) " " x2 "," (:y b) "\" fill=\"none\" stroke=\"hsl(" (mod (* i 37) 360) " 70% 55%)\" stroke-opacity=\"0.28\" stroke-width=\"" sw "\"><title>" (esc (:source e)) " → " (esc (:target e)) ": count " (fmt-num (edge-count e)) ", total " (fmt-time (edge-value e)) "</title></path>\n"))
                            ""))) links))
        node-svg (apply str
                        (map (fn [n]
                               (let [p (get positioned (node-name n))
                                     opacity (+ 0.45 (* 0.55 (sqrt (/ (node-value n) max-node))))]
                                 (str "<g><rect x=\"" (:x p) "\" y=\"" (- (:y p) (/ (:h p) 2))
                                      "\" width=\"" node-w "\" height=\"" (:h p)
                                      "\" rx=\"2\" fill=\"#60a5fa\" opacity=\"" opacity "\"/>"
                                      "<text x=\"" (+ (:x p) node-w 8) "\" y=\"" (- (:y p) 3)
                                      "\" fill=\"#e5e7eb\" font-size=\"11\">" (esc (node-name n)) "</text>"
                                      "<text x=\"" (+ (:x p) node-w 8) "\" y=\"" (+ (:y p) 11)
                                      "\" fill=\"#9ca3af\" font-size=\"10\">count " (fmt-num (node-count n))
                                      " · total " (fmt-time (node-value n)) "</text></g>\n")))
                             nodes))]
    (str "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"" width "\" height=\"" height "\" viewBox=\"0 0 " width " " height "\">"
         "<rect width=\"100%\" height=\"100%\" fill=\"#111827\"/>"
         "<text x=\"" (/ width 2) "\" y=\"34\" fill=\"#f9fafb\" font-size=\"22\" text-anchor=\"middle\" font-family=\"system-ui,sans-serif\">" (esc (:title g)) "</text>"
         "<text x=\"" (/ width 2) "\" y=\"55\" fill=\"#9ca3af\" font-size=\"12\" text-anchor=\"middle\" font-family=\"system-ui,sans-serif\">" (esc (:subtitle g)) "</text>"
         "<g font-family=\"system-ui,sans-serif\">" paths node-svg "</g></svg>")))

(def data (json/read-string (slurp input) {:keywords? true}))
(spit output (render-sankey (trace->graph data)))
(println output)
