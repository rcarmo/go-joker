package core

import "testing"

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
