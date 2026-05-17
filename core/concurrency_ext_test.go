package core

import (
	"sync"
	"testing"
)

func requireKeyword(tb testing.TB, obj Object, want string) {
	tb.Helper()
	got, ok := obj.(Keyword)
	if !ok {
		tb.Fatalf("expected Keyword(%s), got %T (%s)", want, obj, obj.ToString(false))
	}
	if got.ToString(false) != want {
		tb.Fatalf("expected Keyword(%s), got %s", want, got.ToString(false))
	}
}

func TestChannelCloseIsIdempotentUnderConcurrency(t *testing.T) {
	ch := MakeChannel(make(chan FutureResult, 1))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch.Close()
		}()
	}
	wg.Wait()
	if !ch.IsClosed() {
		t.Fatal("channel should report closed after concurrent Close calls")
	}
	if ch.Send(MakeInt(1)) {
		t.Fatal("Send on closed channel should return false")
	}
}

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

	requireString(t, evalTestScript(t, `(let [c (clojure.core.async/chan 1)
        p (promise)]
    (clojure.core.async/take! c #(deliver p %))
    (clojure.core.async/close! c)
    (str @p))`), "")
}

func TestConcurrencyTimeoutRejectsTooLarge(t *testing.T) {
	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		_ = checkedMillisecondDuration(int(^uint(0)>>1), "timeout")
	}()
	if !didPanic {
		t.Fatal("timeout accepted overflowing millisecond value")
	}
}

func TestConcurrencyTimeoutRejectsNegative(t *testing.T) {
	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		_ = evalTestScript(t, `(timeout -1)`)
	}()
	if !didPanic {
		t.Fatal("timeout accepted negative milliseconds")
	}
}

func TestConcurrencyTimeoutAndAltsDefault(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [ch (chan)]
  (first (alts! [ch] :default 42)))`), 42)

	requireKeyword(t, evalTestScript(t, `(let [ch (chan)]
  (second (alts! [ch] :default 42)))`), ":default")
}

func TestConcurrencyAltsRejectsOddOptions(t *testing.T) {
	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		_ = evalTestScript(t, `(let [ch (chan)] (alts! [ch] :default))`)
	}()
	if !didPanic {
		t.Fatal("alts! accepted odd option list")
	}
}

func TestConcurrencyAltsClosedPutReturnsFalse(t *testing.T) {
	requireBool(t, evalTestScript(t, `(let [c (chan)]
  (close! c)
  (first (alts! [[c 1]])))`), false)
}

func TestConcurrencyFuturePromiseAgent(t *testing.T) {
	requireInt(t, evalTestScript(t, `(let [f (future (+ 40 2))] @f)`), 42)
	requireInt(t, evalTestScript(t, `(let [p (promise)] (deliver p 7) @p)`), 7)
	requireInt(t, evalTestScript(t, `(let [a (agent 0)]
  (send a + 10)
  (send a + 20)
  (send a + 12)
  (await a)
  @a)`), 42)
}

func TestConcurrencyPmapAndPcalls(t *testing.T) {
	requireInt(t, evalTestScript(t, `(reduce + 0 (pmap inc [1 2 3 4]))`), 14)
	requireInt(t, evalTestScript(t, `(reduce + 0 (pcalls #(+ 1 1) #(+ 2 2) #(+ 3 3)))`), 12)
}

func TestConcurrencyPcallsRecursiveFn(t *testing.T) {
	requireInt(t, evalTestScript(t, `(letfn [(fib [n]
  (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2)))))]
  (reduce + 0 (pcalls (fn [] (fib 20))
                      (fn [] (fib 20))
                      (fn [] (fib 20)))))`), 20295)
}

func TestConcurrencyPcallsPanicPropagates(t *testing.T) {
	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		_ = evalTestScript(t, `(pcalls (fn [] 1) (fn [] (/ 1 0)))`)
	}()
	if !didPanic {
		t.Fatalf("expected panic to propagate from pcalls worker")
	}
}
