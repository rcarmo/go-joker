package core

import "testing"

const jsonCursorParserScript = `
(let [ws? (fn [c] (or (= c \space) (= c \newline) (= c \tab) (= c \return)))
      skip-ws (fn [cur]
        (loop [c cur]
          (if (cursor-done? c) c
            (if (ws? (cursor-char c)) (recur (cursor-next c)) c))))
      digit? (fn [c] (and (>= (int c) 48) (<= (int c) 57)))]
  (let [parse-string (fn [cur]
          (loop [c (cursor-next cur) buf ""]
            (let [ch (cursor-char c)]
              (if (= ch \") [buf (cursor-next c)]
                (if (= ch \\) (recur (cursor-next (cursor-next c)) (str buf (cursor-char (cursor-next c))))
                  (recur (cursor-next c) (str buf ch)))))))
        parse-number (fn [cur]
          (let [ch (cursor-char cur)
                neg (= ch \-)
                c (if neg (cursor-next cur) cur)]
            (loop [c2 c n 0]
              (if (cursor-done? c2) [(if neg (- 0 n) n) c2]
                (let [ch2 (cursor-char c2)]
                  (if (digit? ch2)
                    (recur (cursor-next c2) (+ (* n 10) (- (int ch2) 48)))
                    [(if neg (- 0 n) n) c2]))))))]
    (let [pv-ref (atom nil)
          parse-array (fn [cur]
            (let [pv @pv-ref
                  c2 (skip-ws (cursor-next cur))]
              (if (= (cursor-char c2) \])
                [[] (cursor-next c2)]
                (loop [c3 c2 arr []]
                  (let [[val nc] (pv c3)
                        nc2 (skip-ws nc)]
                    (if (= (cursor-char nc2) \])
                      [(conj arr val) (cursor-next nc2)]
                      (recur (skip-ws (cursor-next nc2)) (conj arr val))))))))
          parse-object (fn [cur]
            (let [pv @pv-ref
                  c2 (skip-ws (cursor-next cur))]
              (if (= (cursor-char c2) \})
                [{} (cursor-next c2)]
                (loop [c3 c2 m {}]
                  (let [[key nc] (parse-string c3)
                        nc2 (skip-ws nc)
                        nc3 (skip-ws (cursor-next nc2))
                        [val nc4] (pv nc3)
                        nc5 (skip-ws nc4)]
                    (if (= (cursor-char nc5) \})
                      [(assoc m key val) (cursor-next nc5)]
                      (recur (skip-ws (cursor-next nc5)) (assoc m key val))))))))]
      (let [parse-value (fn [cur]
              (let [c2 (skip-ws cur)
                    ch (cursor-char c2)]
                (if (= ch \") (parse-string c2)
                  (if (= ch \{) (parse-object c2)
                    (if (= ch \[) (parse-array c2)
                      (if (= ch \t) [true (cursor-next (cursor-next (cursor-next (cursor-next c2))))]
                        (if (= ch \f) [false (cursor-next (cursor-next (cursor-next (cursor-next (cursor-next c2)))))]
                          (if (= ch \n) [nil (cursor-next (cursor-next (cursor-next (cursor-next c2))))]
                            (parse-number c2)))))))))
            _ (reset! pv-ref parse-value)]
        (fn [s] (first (parse-value (string-cursor s))))))))
`

func getCursorJSONParser(tb testing.TB) Callable {
	initStringCursorProcs()
	return Eval(compileBenchExpr(tb, jsonCursorParserScript), nil).(Callable)
}

func TestCursorJSONCorrectness(t *testing.T) {
	parse := getCursorJSONParser(t)
	r1 := parse.Call([]Object{String{S: `42`}})
	if r1.(Int).I != 42 { t.Fatalf("expected 42, got %v", r1) }
	r2 := parse.Call([]Object{String{S: `"hello"`}})
	if r2.(String).S != "hello" { t.Fatalf("expected hello, got %v", r2) }
	r3 := parse.Call([]Object{String{S: `[1,2,3]`}})
	if r3 == nil { t.Fatal("array parse failed") }
	r4 := parse.Call([]Object{String{S: jsonSmall}})
	if r4 == nil { t.Fatal("object parse failed") }
	t.Logf("small JSON: %s", r4.ToString(false)[:50])
}

func BenchmarkCursorParseJSONSmall(b *testing.B) {
	parse := getCursorJSONParser(b)
	input := String{S: jsonSmall}
	b.ResetTimer()
	for i := 0; i < b.N; i++ { parse.Call([]Object{input}) }
}

func BenchmarkCursorParseJSONMedium(b *testing.B) {
	parse := getCursorJSONParser(b)
	input := String{S: jsonMedium}
	b.ResetTimer()
	for i := 0; i < b.N; i++ { parse.Call([]Object{input}) }
}
