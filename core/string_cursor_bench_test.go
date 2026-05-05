package core

import "testing"

// Benchmark comparing string iteration with nth vs cursor
func BenchmarkStringIterNth(b *testing.B) {
	initStringCursorProcs() // ensure procs available
	// Count chars in string using (loop [i 0 c 0] (if (= i len) c (recur (+ i 1) (+ c 1))))
	// with nth to verify each char exists
	script := `(let [s "The quick brown fox jumps over the lazy dog and does many other things"
                  len (count s)]
              (loop [i 0 c 0]
                (if (= i len)
                  c
                  (let [ch (nth s i)]
                    (recur (+ i 1) (if (= ch \space) (+ c 1) c))))))`
	expr := compileBenchExpr(b, script)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Eval(expr, nil)
	}
}

func BenchmarkStringIterCursor(b *testing.B) {
	initStringCursorProcs()
	// Same but with cursor
	script := `(let [s "The quick brown fox jumps over the lazy dog and does many other things"
                  cur (string-cursor s)]
              (loop [c cur spaces 0]
                (if (cursor-done? c)
                  spaces
                  (let [ch (cursor-char c)]
                    (recur (cursor-next c) (if (= ch \space) (+ spaces 1) spaces))))))`
	expr := compileBenchExpr(b, script)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Eval(expr, nil)
	}
}
