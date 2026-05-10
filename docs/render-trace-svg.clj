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
  (-> (str s)
      (str/replace "&" "&amp;")
      (str/replace "<" "&lt;")
      (str/replace ">" "&gt;")
      (str/replace "\"" "&quot;")))

(defn fmt-num [n]
  (cond
    (>= n 1000000000) (str (format "%.1f" (/ n 1000000000.0)) "G")
    (>= n 1000000) (str (format "%.1f" (/ n 1000000.0)) "M")
    (>= n 1000) (str (format "%.1f" (/ n 1000.0)) "K")
    :else (str n)))

(defn row-name [row]
  (or (:name row) (:symbol row) (:source row) "?"))

(defn row-count [row]
  (or (:count row) (:value row) (:samples row) 0))

(defn select-rows [data]
  (cond
    (:nodes data) {:title (or explicit-title (:title data) "Go pprof trace")
                   :subtitle "Top pprof Sankey nodes from shared nodes/links JSON"
                   :rows (take 24 (:nodes data))
                   :value-label "sampled time"}
    (= (:type data) "go-joker-ir-profile") {:title (or explicit-title "Joker IR opcode trace")
                                             :subtitle (str "IR executions " (:execs data))
                                             :rows (take 24 (:ops data))
                                             :value-label "opcode count"}
    (= (:type data) "go-joker-function-trace") {:title (or explicit-title "Joker function trace")
                                                :subtitle (str "Function calls " (:total data))
                                                :rows (take 24 (:functions data))
                                                :value-label "call count"}
    (= (:type data) "go-joker-symbol-trace") {:title (or explicit-title "Joker symbol trace")
                                              :subtitle (str "resolves " (:resolve_total data) " · derefs " (:deref_total data))
                                              :rows (concat (take 12 (:resolves data)) (take 12 (:derefs data)))
                                              :value-label "count"}
    :else {:title (or explicit-title "Trace") :subtitle "Unknown trace shape" :rows [] :value-label "count"}))

(defn render-svg [selected]
  (let [rows (:rows selected)
        width 1200
        row-h 28
        top 82
        height (+ top 40 (* row-h (count rows)))
        max-v (max 1 (reduce max 1 (map row-count rows)))]
    (str "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"" width "\" height=\"" height "\" viewBox=\"0 0 " width " " height "\">\n"
         "<rect width=\"100%\" height=\"100%\" fill=\"#111827\"/>\n"
         "<text x=\"" (/ width 2) "\" y=\"34\" fill=\"#f9fafb\" font-size=\"22\" text-anchor=\"middle\" font-family=\"system-ui,sans-serif\">" (esc (:title selected)) "</text>\n"
         "<text x=\"" (/ width 2) "\" y=\"56\" fill=\"#9ca3af\" font-size=\"12\" text-anchor=\"middle\" font-family=\"system-ui,sans-serif\">" (esc (:subtitle selected)) "</text>\n"
         "<text x=\"420\" y=\"76\" fill=\"#64748b\" font-size=\"11\" font-family=\"system-ui,sans-serif\">" (esc (:value-label selected)) "</text>\n"
         (apply str
                (map-indexed
                 (fn [i row]
                   (let [y (+ top (* i row-h))
                         v (row-count row)
                         bar-w (* 700 (/ v max-v))
                         color (if (even? i) "#60a5fa" "#38bdf8")]
                     (str "<text x=\"24\" y=\"" (+ y 15) "\" fill=\"#e5e7eb\" font-size=\"12\" font-family=\"system-ui,sans-serif\">" (esc (row-name row)) "</text>\n"
                          "<rect x=\"420\" y=\"" y "\" width=\"" bar-w "\" height=\"18\" rx=\"4\" fill=\"" color "\" opacity=\"0.82\"/>\n"
                          "<text x=\"" (+ 430 bar-w) "\" y=\"" (+ y 14) "\" fill=\"#cbd5e1\" font-size=\"11\" font-family=\"system-ui,sans-serif\">" (fmt-num v) "</text>\n")))
                 rows))
         "</svg>\n")))

(def data (json/read-string (slurp input) {:keywords? true}))
(spit output (render-svg (select-rows data)))
(println output)
