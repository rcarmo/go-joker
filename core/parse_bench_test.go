package core

import "testing"

// Pure-Clojure JSON parser (integers only, no floats).
// Split into small nested lets to avoid Go stack overflow.
const jsonParserScript = `
(let [ws? (fn [c] (or (= c \space) (= c \newline) (= c \tab) (= c \return)))
      skip-ws (fn [s i]
        (loop [j i]
          (if (>= j (count s)) j
            (if (ws? (nth s j)) (recur (+ j 1)) j))))
      digit? (fn [c] (and (>= (int c) 48) (<= (int c) 57)))]
  (let [parse-string (fn [s i]
          (loop [j (+ i 1) buf ""]
            (let [c (nth s j)]
              (if (= c \") [buf (+ j 1)]
                (if (= c \\) (recur (+ j 2) (str buf (nth s (+ j 1))))
                  (recur (+ j 1) (str buf c)))))))
        parse-number (fn [s i]
          (let [neg (= (nth s i) \-)
                start (if neg (+ i 1) i)]
            (loop [j start n 0]
              (if (>= j (count s)) [(if neg (- 0 n) n) j]
                (if (digit? (nth s j))
                  (recur (+ j 1) (+ (* n 10) (- (int (nth s j)) 48)))
                  [(if neg (- 0 n) n) j])))))]
    (let [pv-ref (atom nil)
          parse-array (fn [s i]
            (let [pv @pv-ref
                  i2 (skip-ws s (+ i 1))]
              (if (= (nth s i2) \])
                [[] (+ i2 1)]
                (loop [i3 i2 arr []]
                  (let [[val ni] (pv s i3)
                        ni2 (skip-ws s ni)]
                    (if (= (nth s ni2) \])
                      [(conj arr val) (+ ni2 1)]
                      (recur (skip-ws s (+ ni2 1)) (conj arr val))))))))
          parse-object (fn [s i]
            (let [pv @pv-ref
                  i2 (skip-ws s (+ i 1))]
              (if (= (nth s i2) \})
                [{} (+ i2 1)]
                (loop [i3 i2 m {}]
                  (let [[key ni] (parse-string s i3)
                        ni2 (skip-ws s ni)
                        ni3 (skip-ws s (+ ni2 1))
                        [val ni4] (pv s ni3)
                        ni5 (skip-ws s ni4)]
                    (if (= (nth s ni5) \})
                      [(assoc m key val) (+ ni5 1)]
                      (recur (skip-ws s (+ ni5 1)) (assoc m key val))))))))]
      (let [parse-value (fn [s i]
              (let [i2 (skip-ws s i)
                    c (nth s i2)]
                (if (= c \") (parse-string s i2)
                  (if (= c \{) (parse-object s i2)
                    (if (= c \[) (parse-array s i2)
                      (if (= c \t) [true (+ i2 4)]
                        (if (= c \f) [false (+ i2 5)]
                          (if (= c \n) [nil (+ i2 4)]
                            (parse-number s i2)))))))))
            _ (reset! pv-ref parse-value)]
        (fn [s] (first (parse-value s 0)))))))
`

const jsonSmall = `{"name":"John","age":30,"city":"New York","active":true,"scores":[95,87,92]}`
const jsonMedium = `[{"id":1,"name":"Alice","email":"alice@test.com","tags":["admin","user"],"score":95},{"id":2,"name":"Bob","email":"bob@test.com","tags":["user"],"score":87},{"id":3,"name":"Charlie","email":"charlie@test.com","tags":["user","mod"],"score":92},{"id":4,"name":"Dave","email":"dave@test.com","tags":[],"score":78},{"id":5,"name":"Eve","email":"eve@test.com","tags":["admin"],"score":99}]`

func getJSONParser(tb testing.TB) Callable {
	clbgInit()
	return Eval(compileBenchExpr(tb, jsonParserScript), nil).(Callable)
}

func TestJSONParserCorrectness(t *testing.T) {
	parse := getJSONParser(t)
	tests := []struct{ input, want string }{
		{`42`, "42"},
		{`-7`, "-7"},
		{`"hello"`, "hello"},
		{`true`, "true"},
		{`false`, "false"},
		{`null`, "nil"},
		{`[1,2,3]`, "[1 2 3]"},
		{`{"a":1}`, `{a 1}`},
	}
	for _, tt := range tests {
		r := parse.Call([]Object{String{S: tt.input}})
		if r == nil {
			t.Fatalf("parse(%q) = nil", tt.input)
		}
		got := r.ToString(false)
		if got != tt.want {
			t.Errorf("parse(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
	r := parse.Call([]Object{String{S: jsonSmall}})
	if r == nil { t.Fatal("small JSON failed") }
	t.Logf("small: %s", r.ToString(false))
	r2 := parse.Call([]Object{String{S: jsonMedium}})
	if r2 == nil { t.Fatal("medium JSON failed") }
	t.Logf("medium: %d items", r2.(*ArrayVector).Count())
}

func BenchmarkParseJSONSmall(b *testing.B) {
	parse := getJSONParser(b)
	input := String{S: jsonSmall}
	b.ResetTimer()
	for i := 0; i < b.N; i++ { parse.Call([]Object{input}) }
}

func BenchmarkParseJSONMedium(b *testing.B) {
	parse := getJSONParser(b)
	input := String{S: jsonMedium}
	b.ResetTimer()
	for i := 0; i < b.N; i++ { parse.Call([]Object{input}) }
}
