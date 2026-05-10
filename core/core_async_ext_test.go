package core

import "testing"

func TestCoreAsyncNamespaceAliases(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [c (clojure.core.async/chan 1)]
    (clojure.core.async/>!! c 42)
    (clojure.core.async/<!! c))`), 42)
}

func TestCoreAsyncGoLoopAndPipeline(t *testing.T) {
	requireString(t, evalTestScript(t, `(let [c (clojure.core.async/chan 1)]
    (clojure.core.async/go-loop [i 0]
      (if (< i 3)
        (do (clojure.core.async/>! c i) (recur (inc i)))
        (clojure.core.async/close! c)))
    (str (clojure.core.async/<!! c) ":" (clojure.core.async/<!! c) ":" (clojure.core.async/<!! c) ":" (clojure.core.async/<!! c)))`), "0:1:2:")
}

func TestCoreAsyncMapFilterMergeSplit(t *testing.T) {
	requireString(t, evalTestScript(t, `(let [mapped (clojure.core.async/map< inc (clojure.core.async/to-chan [1 2 3]))
        filtered (clojure.core.async/filter< even? mapped)]
    (str (clojure.core.async/<!! filtered) ":" (clojure.core.async/<!! filtered)))`), "2:4")

	requireString(t, evalTestScript(t, `(let [m (clojure.core.async/merge [(clojure.core.async/to-chan [1]) (clojure.core.async/to-chan [2])])
        xs [(clojure.core.async/<!! m) (clojure.core.async/<!! m)]]
    (str (count (set xs)) ":" (contains? (set xs) 1) ":" (contains? (set xs) 2)))`), "2:true:true")

	requireString(t, evalTestScript(t, `(let [[evens odds] (clojure.core.async/split even? (clojure.core.async/to-chan [1 2]))]
    (str (clojure.core.async/<!! odds) ":" (clojure.core.async/<!! evens)))`), "1:2")
}

func TestCoreAsyncMultAndPub(t *testing.T) {
	requireString(t, evalTestScript(t, `(let [src (clojure.core.async/chan 1)
        m (clojure.core.async/mult src)
        t1 (clojure.core.async/chan 1)
        t2 (clojure.core.async/chan 1)]
    (clojure.core.async/tap m t1)
    (clojure.core.async/tap m t2)
    (clojure.core.async/>!! src :x)
    (str (clojure.core.async/<!! t1) ":" (clojure.core.async/<!! t2)))`), ":x::x")

	requireString(t, evalTestScript(t, `(let [src (clojure.core.async/chan 1)
        p (clojure.core.async/pub src identity)
        out (clojure.core.async/chan 1)]
    (clojure.core.async/sub p :topic out)
    (clojure.core.async/>!! src :topic)
    (str (clojure.core.async/<!! out)))`), ":topic")
}

func TestCoreAsyncReduceIntoAndCallbacks(t *testing.T) {
	requireInt(t, evalTestScript(t, `(clojure.core.async/<!! (clojure.core.async/reduce + 0 (clojure.core.async/to-chan [1 2 3])))`), 6)

	requireString(t, evalTestScript(t, `(str (clojure.core.async/<!! (clojure.core.async/into [] (clojure.core.async/to-chan [1 2]))))`), "[1 2]")

	requireString(t, evalTestScript(t, `(let [c (clojure.core.async/chan 1)
        p (promise)]
    (clojure.core.async/take! c #(deliver p %))
    (clojure.core.async/put! c 9)
    (str @p))`), "9")
}
