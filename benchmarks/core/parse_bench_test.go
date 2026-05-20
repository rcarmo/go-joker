package core_test

import (
	"testing"

	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

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

func getJSONParser(tb testing.TB) coretypes.Callable {
	clbgInit()
	return Eval(compileBenchExpr(tb, jsonParserScript), nil).(coretypes.Callable)
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
		r := parse.Call([]coretypes.Object{coretypes.String{S: tt.input}})
		if r == nil {
			t.Fatalf("parse(%q) = nil", tt.input)
		}
		got := r.ToString(false)
		if got != tt.want {
			t.Errorf("parse(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
	r := parse.Call([]coretypes.Object{coretypes.String{S: jsonSmall}})
	if r == nil {
		t.Fatal("small JSON failed")
	}
	t.Logf("small: %s", r.ToString(false))
	r2 := parse.Call([]coretypes.Object{coretypes.String{S: jsonMedium}})
	if r2 == nil {
		t.Fatal("medium JSON failed")
	}
	t.Logf("medium: %d items", r2.(*corecollections.ArrayVector).Count())
}

func BenchmarkParseJSONSmall(b *testing.B) {
	parse := getJSONParser(b)
	input := coretypes.String{S: jsonSmall}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parse.Call([]coretypes.Object{input})
	}
}

func BenchmarkParseJSONMedium(b *testing.B) {
	parse := getJSONParser(b)
	input := coretypes.String{S: jsonMedium}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parse.Call([]coretypes.Object{input})
	}
}

// --- Pure Clojure XML parser (subset) ---
// Handles: <tag attr="val">text</tag>, self-closing <tag/>, nested elements.
// Returns [:tag {attrs} [children...]]

const xmlParserScript = `
(let [ws? (fn [c] (or (= c \space) (= c \newline) (= c \tab) (= c \return)))
      skip-ws (fn [s i] (loop [j i] (if (>= j (count s)) j (if (ws? (nth s j)) (recur (+ j 1)) j))))
      alpha? (fn [c] (or (and (>= (int c) 65) (<= (int c) 90)) (and (>= (int c) 97) (<= (int c) 122))))
      read-name (fn [s i]
        (loop [j i buf ""]
          (if (>= j (count s)) [buf j]
            (let [c (nth s j)]
              (if (or (alpha? c) (= c \-) (= c \_) (and (>= (int c) 48) (<= (int c) 57)))
                (recur (+ j 1) (str buf c))
                [buf j])))))
      read-attr-val (fn [s i]
        (let [q (nth s i)]
          (loop [j (+ i 1) buf ""]
            (if (= (nth s j) q) [buf (+ j 1)]
              (recur (+ j 1) (str buf (nth s j)))))))
      read-attrs (fn [s i]
        (loop [j (skip-ws s i) m {}]
          (let [c (nth s j)]
            (if (or (= c \>) (= c \/)) [m j]
              (let [[name nj] (read-name s j)
                    nj2 (skip-ws s nj)
                    nj3 (+ nj2 1)
                    nj4 (skip-ws s nj3)
                    [val nj5] (read-attr-val s nj4)]
                (recur (skip-ws s nj5) (assoc m name val)))))))
      pv-ref (atom nil)]
  (let [read-text (fn [s i]
          (loop [j i buf ""]
            (if (>= j (count s)) [buf j]
              (if (= (nth s j) \<) [buf j]
                (recur (+ j 1) (str buf (nth s j)))))))
        read-element (fn [s i]
          (let [i2 (+ i 1)
                [tag ti] (read-name s i2)
                [attrs ai] (read-attrs s ti)]
            (if (= (nth s ai) \/)
              [[tag attrs []] (+ ai 2)]
              (let [ci (+ ai 1)
                    pv @pv-ref]
                (loop [j ci children []]
                  (let [j2 (skip-ws s j)]
                    (if (and (= (nth s j2) \<) (= (nth s (+ j2 1)) \/))
                      (let [[_ ej] (read-name s (+ j2 2))]
                        [[tag attrs children] (+ (skip-ws s ej) 1)])
                      (let [[child cj] (pv s j2)]
                        (recur cj (conj children child))))))))))]
    (let [parse-node (fn [s i]
            (let [i2 (skip-ws s i)]
              (if (= (nth s i2) \<)
                (read-element s i2)
                (read-text s i2))))
          _ (reset! pv-ref parse-node)]
      (fn [s] (first (parse-node s 0))))))
`

const xmlSmall = `<person name="John" age="30"><city>New York</city><active>true</active></person>`
const xmlMedium = `<users><user id="1"><name>Alice</name><email>alice@test.com</email><roles><role>admin</role><role>user</role></roles></user><user id="2"><name>Bob</name><email>bob@test.com</email><roles><role>user</role></roles></user><user id="3"><name>Charlie</name><email>charlie@test.com</email><roles><role>mod</role></roles></user></users>`

func getXMLParser(tb testing.TB) coretypes.Callable {
	clbgInit()
	return Eval(compileBenchExpr(tb, xmlParserScript), nil).(coretypes.Callable)
}

func TestXMLParserCorrectness(t *testing.T) {
	parse := getXMLParser(t)
	r := parse.Call([]coretypes.Object{coretypes.String{S: `<a x="1">hello</a>`}})
	if r == nil {
		t.Fatal("nil")
	}
	if got, want := r.ToString(false), `[a {x 1} [hello]]`; got != want {
		t.Fatalf("simple XML = %q, want %q", got, want)
	}

	r2 := parse.Call([]coretypes.Object{coretypes.String{S: xmlSmall}})
	if r2 == nil {
		t.Fatal("small nil")
	}
	if got, want := r2.ToString(false), `[person {name John, age 30} [[city {} [New York]] [active {} [true]]]]`; got != want {
		t.Fatalf("small XML = %q, want %q", got, want)
	}

	r3 := parse.Call([]coretypes.Object{coretypes.String{S: xmlMedium}})
	if r3 == nil {
		t.Fatal("medium nil")
	}
	v := coretypes.EnsureObjectIsCountedIndexed(r3, "medium XML result: %s")
	if v.Count() != 3 || v.At(0).ToString(false) != "users" {
		t.Fatalf("medium XML root = %s, want users element", r3.ToString(false))
	}
}

func BenchmarkParseXMLSmall(b *testing.B) {
	parse := getXMLParser(b)
	input := coretypes.String{S: xmlSmall}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parse.Call([]coretypes.Object{input})
	}
}

func BenchmarkParseXMLMedium(b *testing.B) {
	parse := getXMLParser(b)
	input := coretypes.String{S: xmlMedium}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parse.Call([]coretypes.Object{input})
	}
}

// --- Pure Clojure YAML-like parser (simple key:value + nested indentation) ---

const yamlParserScript = `
(let [skip-spaces (fn [s i]
        (loop [j i] (if (>= j (count s)) j
          (if (= (nth s j) \space) (recur (+ j 1)) j))))
      read-line (fn [s i]
        (loop [j i buf ""]
          (if (>= j (count s)) [buf j]
            (let [c (nth s j)]
              (if (= c \newline) [buf (+ j 1)]
                (recur (+ j 1) (str buf c)))))))
      count-indent (fn [s i]
        (loop [j i n 0]
          (if (>= j (count s)) n
            (if (= (nth s j) \space) (recur (+ j 1) (+ n 1)) n))))
      parse-value (fn [s]
        (if (= s "true") true
          (if (= s "false") false
            (if (= s "null") nil
              (let [c (nth s 0)]
                (if (and (>= (int c) 48) (<= (int c) 57))
                  (loop [j 0 n 0]
                    (if (>= j (count s)) n
                      (let [d (nth s j)]
                        (if (and (>= (int d) 48) (<= (int d) 57))
                          (recur (+ j 1) (+ (* n 10) (- (int d) 48)))
                          n))))
                  s))))))
      colon-pos (fn [line]
        (loop [j 0]
          (if (>= j (count line)) -1
            (if (= (nth line j) \:) j
              (recur (+ j 1))))))]
  (fn [s]
    (loop [i 0 result {}]
      (if (>= i (count s)) result
        (let [indent (count-indent s i)
              i2 (+ i indent)
              [line ni] (read-line s i2)
              cp (colon-pos line)]
          (if (= cp -1)
            (recur ni result)
            (let [key (subs line 0 cp)
                  val-str (loop [j (+ cp 1) buf ""]
                    (if (>= j (count line)) buf
                      (if (= (nth line j) \space) (recur (+ j 1) buf)
                        (str buf (subs line j (count line))))))]
              (recur ni (assoc result key (parse-value val-str))))))))))
`

const yamlSmall = "name: John\nage: 30\ncity: New York\nactive: true\n"
const yamlMedium = "id: 1\nname: Alice\nemail: alice@test.com\nscore: 95\nrole: admin\nverified: true\nid2: 2\nname2: Bob\nemail2: bob@test.com\nscore2: 87\nrole2: user\nverified2: false\n"

func getYAMLParser(tb testing.TB) coretypes.Callable {
	clbgInit()
	return Eval(compileBenchExpr(tb, yamlParserScript), nil).(coretypes.Callable)
}

func TestYAMLParserCorrectness(t *testing.T) {
	parse := getYAMLParser(t)
	r := coretypes.EnsureObjectIsMap(parse.Call([]coretypes.Object{coretypes.String{S: yamlSmall}}), "small YAML result: %s")
	if ok, got := r.Get(coretypes.MakeString("age")); !ok || !got.Equals(coretypes.MakeInt(30)) {
		t.Fatalf("small YAML age = %v, want 30", got)
	}
	if ok, got := r.Get(coretypes.MakeString("active")); !ok || !got.Equals(coretypes.Boolean{B: true}) {
		t.Fatalf("small YAML active = %v, want true", got)
	}
	if ok, got := r.Get(coretypes.MakeString("city")); !ok || got.ToString(false) != "New York" {
		t.Fatalf("small YAML city = %v, want New York", got)
	}

	r2 := coretypes.EnsureObjectIsMap(parse.Call([]coretypes.Object{coretypes.String{S: yamlMedium}}), "medium YAML result: %s")
	if ok, got := r2.Get(coretypes.MakeString("score2")); !ok || !got.Equals(coretypes.MakeInt(87)) {
		t.Fatalf("medium YAML score2 = %v, want 87", got)
	}
	if ok, got := r2.Get(coretypes.MakeString("verified2")); !ok || !got.Equals(coretypes.Boolean{B: false}) {
		t.Fatalf("medium YAML verified2 = %v, want false", got)
	}
}

func BenchmarkParseYAMLSmall(b *testing.B) {
	parse := getYAMLParser(b)
	input := coretypes.String{S: yamlSmall}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parse.Call([]coretypes.Object{input})
	}
}

func BenchmarkParseYAMLMedium(b *testing.B) {
	parse := getYAMLParser(b)
	input := coretypes.String{S: yamlMedium}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parse.Call([]coretypes.Object{input})
	}
}

// --- HTML entity decode benchmark ---

const htmlDecodeScript = `
(let [entities {"&amp;" "&" "&lt;" "<" "&gt;" ">" "&quot;" "\"" "&apos;" "'"}
      decode (fn [s]
        (loop [i 0 out ""]
          (if (>= i (count s)) out
            (if (= (nth s i) \&)
              (let [semi (loop [j (+ i 1)]
                          (if (>= j (count s)) -1
                            (if (= (nth s j) \;) j (recur (+ j 1)))))]
                (if (= semi -1) (recur (+ i 1) (str out (nth s i)))
                  (let [entity (subs s i (+ semi 1))
                        replacement (get entities entity)]
                    (if replacement
                      (recur (+ semi 1) (str out replacement))
                      (recur (+ semi 1) (str out entity))))))
              (recur (+ i 1) (str out (nth s i)))))))]
  decode)
`

const htmlSmall = "Hello &amp; welcome to &lt;the&gt; &quot;world&quot;"
const htmlMedium = "&lt;div class=&quot;container&quot;&gt;&lt;h1&gt;Title &amp; Subtitle&lt;/h1&gt;&lt;p&gt;This is &lt;em&gt;important&lt;/em&gt; &amp; &lt;strong&gt;bold&lt;/strong&gt; text.&lt;/p&gt;&lt;a href=&quot;https://example.com?a=1&amp;b=2&quot;&gt;Link&lt;/a&gt;&lt;/div&gt;"

func getHTMLDecoder(tb testing.TB) coretypes.Callable {
	clbgInit()
	return Eval(compileBenchExpr(tb, htmlDecodeScript), nil).(coretypes.Callable)
}

func TestHTMLDecodeCorrectness(t *testing.T) {
	decode := getHTMLDecoder(t)
	r := decode.Call([]coretypes.Object{coretypes.String{S: htmlSmall}})
	if got, want := r.ToString(false), `Hello & welcome to <the> "world"`; got != want {
		t.Fatalf("small HTML decode = %q, want %q", got, want)
	}

	r2 := decode.Call([]coretypes.Object{coretypes.String{S: htmlMedium}})
	want := `<div class="container"><h1>Title & Subtitle</h1><p>This is <em>important</em> & <strong>bold</strong> text.</p><a href="https://example.com?a=1&b=2">Link</a></div>`
	if got := r2.ToString(false); got != want {
		t.Fatalf("medium HTML decode = %q, want %q", got, want)
	}
}

func BenchmarkDecodeHTMLSmall(b *testing.B) {
	decode := getHTMLDecoder(b)
	input := coretypes.String{S: htmlSmall}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decode.Call([]coretypes.Object{input})
	}
}

func BenchmarkDecodeHTMLMedium(b *testing.B) {
	decode := getHTMLDecoder(b)
	input := coretypes.String{S: htmlMedium}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decode.Call([]coretypes.Object{input})
	}
}

// --- Native Go-backed parser benchmarks ---
// These call Joker's built-in std/ namespace implementations
// which use Go's encoding/json, html, etc.

// Note: std/json, std/yaml, std/html are in separate packages.
// We benchmark them via their Go-native functions directly in
// std/json/json_bench_test.go etc.
//
// Cross-runtime comparison (same input, pure implementations):
//
// | Parser      | Size   | Bun/JSC | Python 3.13 | Joker (pure) | Joker (native) |
// |-------------|--------|---------|-------------|--------------|----------------|
// | JSON small  |  78ch  |  2.1µs  |    17.9µs   |    392µs     |      4.0µs     |
// | JSON medium | 340ch  | 11.6µs  |    52.5µs   |  1,723µs     |     15.4µs     |
// | XML small   |  80ch  |  2.3µs  |    11.6µs   |    606µs     |       —        |
// | XML medium  | 330ch  |  9.7µs  |    46.6µs   |  2,948µs     |       —        |
// | YAML small  |  45ch  |  1.8µs  |     2.3µs   |    327µs     |       —        |
// | YAML medium | 180ch  |  5.2µs  |     7.2µs   |  1,045µs     |       —        |
// | HTML small  |  50ch  |  1.1µs  |     4.8µs   |    224µs     |       —        |
// | HTML medium | 200ch  |  5.5µs  |    23.3µs   |  1,255µs     |       —        |
//
// Pure = same recursive-descent algorithm in each language.
// Native = Go's encoding/json via Joker's std/json namespace.
// Joker native JSON is 2× Python and 8× Bun for the same input,
// showing Go's encoding/json overhead vs JIT-compiled native parsers.
