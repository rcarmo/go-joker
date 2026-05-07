//go:build ignore

package main

// clojure_parity_test.go — Direct Clojure parity tests for go-joker.
//
// Tests core Clojure forms by evaluating expressions and checking results.
// Does not depend on clojure.test or any external test framework.
//
// Usage:
//   go run tests/clojure_parity_test.go [-joker path] [-out report.md]

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type PTest struct {
	Category string
	Name     string
	Expr     string // expression to eval
	Expected string // expected output (trimmed)
}

type PResult struct {
	Category string
	Name     string
	Status   string // pass, fail, error
	Got      string
	Expected string
}

// Tests organized by category
var parityTests = []PTest{
	// --- Arithmetic ---
	{"arithmetic", "add-ints", "(+ 1 2 3)", "6"},
	{"arithmetic", "add-mixed", "(+ 1 2.0)", "3.0"},
	{"arithmetic", "sub", "(- 10 3)", "7"},
	{"arithmetic", "mul", "(* 3 4)", "12"},
	{"arithmetic", "div-int", "(/ 10 3)", "3.3333333333333335"},
	{"arithmetic", "rem", "(rem 10 3)", "1"},
	{"arithmetic", "mod", "(mod 10 3)", "1"},
	{"arithmetic", "inc", "(inc 5)", "6"},
	{"arithmetic", "dec", "(dec 5)", "4"},
	{"arithmetic", "max", "(max 1 5 3)", "5"},
	{"arithmetic", "min", "(min 1 5 3)", "1"},
	{"arithmetic", "abs", "(abs -5)", "5"},

	// --- Comparisons ---
	{"comparison", "lt", "(< 1 2)", "true"},
	{"comparison", "gt", "(> 2 1)", "true"},
	{"comparison", "lte", "(<= 2 2)", "true"},
	{"comparison", "gte", "(>= 3 2)", "true"},
	{"comparison", "eq-num", "(= 1 1)", "true"},
	{"comparison", "eq-str", `(= "a" "a")`, "true"},
	{"comparison", "not-eq", "(not= 1 2)", "true"},
	{"comparison", "zero?", "(zero? 0)", "true"},
	{"comparison", "pos?", "(pos? 1)", "true"},
	{"comparison", "neg?", "(neg? -1)", "true"},
	{"comparison", "even?", "(even? 4)", "true"},
	{"comparison", "odd?", "(odd? 3)", "true"},

	// --- Strings ---
	{"string", "str", `(str "hello" " " "world")`, "hello world"},
	{"string", "count-str", `(count "hello")`, "5"},
	{"string", "subs", `(subs "hello" 1 3)`, "el"},
	{"string", "string?", `(string? "hello")`, "true"},
	{"string", "char", `(char 65)`, `A`},
	{"string", "int-char", `(int \A)`, "65"},

	// --- Collections: vectors ---
	{"vector", "literal", "(vector 1 2 3)", "[1 2 3]"},
	{"vector", "conj", "(conj [1 2] 3)", "[1 2 3]"},
	{"vector", "nth", "(nth [10 20 30] 1)", "20"},
	{"vector", "first", "(first [1 2 3])", "1"},
	{"vector", "rest", "(vec (rest [1 2 3]))", "[2 3]"},
	{"vector", "count", "(count [1 2 3])", "3"},
	{"vector", "empty?-no", "(empty? [1])", "false"},
	{"vector", "empty?-yes", "(empty? [])", "true"},
	{"vector", "assoc", "(assoc [1 2 3] 1 99)", "[1 99 3]"},
	{"vector", "into", "(into [] '(1 2 3))", "[1 2 3]"},
	{"vector", "subvec", "(subvec [1 2 3 4 5] 1 3)", "[2 3]"},
	{"vector", "peek", "(peek [1 2 3])", "3"},
	{"vector", "pop", "(pop [1 2 3])", "[1 2]"},

	// --- Collections: maps ---
	{"map", "literal", "(hash-map :a 1 :b 2)", "{:b 2, :a 1}"},
	{"map", "assoc", "(:b (assoc {:a 1} :b 2))", "2"},
	{"map", "dissoc", "(dissoc {:a 1 :b 2} :a)", "{:b 2}"},
	{"map", "get", "(get {:a 1} :a)", "1"},
	{"map", "get-default", "(get {:a 1} :b 42)", "42"},
	{"map", "contains?", "(contains? {:a 1} :a)", "true"},
	{"map", "keys", "(sort (keys {:b 2 :a 1}))", "(:a :b)"},
	{"map", "vals", "(sort (vals {:a 1 :b 2}))", "(1 2)"},
	{"map", "merge", "(:b (merge {:a 1} {:b 2}))", "2"},
	{"map", "select-keys", "(select-keys {:a 1 :b 2 :c 3} [:a :c])", "{:a 1, :c 3}"},
	{"map", "count", "(count {:a 1 :b 2})", "2"},
	{"map", "map?", "(map? {:a 1})", "true"},

	// --- Collections: sets ---
	{"set", "literal", "(count (hash-set 3 1 2))", "3"},
	{"set", "conj", "(contains? (conj #{1 2} 3) 3)", "true"},
	{"set", "disj", "(disj #{1 2 3} 2)", "#{1 3}"},
	{"set", "contains?", "(contains? #{1 2 3} 2)", "true"},
	{"set", "count", "(count #{1 2 3})", "3"},
	{"set", "set?", "(set? #{1})", "true"},

	// --- Collections: lists ---
	{"list", "literal", "(list 1 2 3)", "(1 2 3)"},
	{"list", "cons", "(cons 0 '(1 2 3))", "(0 1 2 3)"},
	{"list", "first", "(first '(1 2 3))", "1"},
	{"list", "rest", "(rest '(1 2 3))", "(2 3)"},
	{"list", "count", "(count '(1 2 3))", "3"},
	{"list", "list?", "(list? '(1 2 3))", "true"},

	// --- Sequences ---
	{"seq", "map", "(vec (map inc [1 2 3]))", "[2 3 4]"},
	{"seq", "filter", "(vec (filter even? [1 2 3 4]))", "[2 4]"},
	{"seq", "reduce", "(reduce + [1 2 3 4])", "10"},
	{"seq", "reduce-init", "(reduce + 10 [1 2 3])", "16"},
	{"seq", "take", "(vec (take 3 (range 10)))", "[0 1 2]"},
	{"seq", "drop", "(vec (drop 3 [1 2 3 4 5]))", "[4 5]"},
	{"seq", "take-while", "(vec (take-while #(< % 4) [1 2 3 4 5]))", "[1 2 3]"},
	{"seq", "drop-while", "(vec (drop-while #(< % 3) [1 2 3 4 5]))", "[3 4 5]"},
	{"seq", "concat", "(vec (concat [1 2] [3 4]))", "[1 2 3 4]"},
	{"seq", "mapcat", "(vec (mapcat #(vector % (* % %)) [1 2 3]))", "[1 1 2 4 3 9]"},
	{"seq", "sort", "(sort [3 1 2])", "(1 2 3)"},
	{"seq", "sort-by", "(sort-by count [\"aaa\" \"b\" \"cc\"])", "(b cc aaa)"},
	{"seq", "reverse", "(vec (reverse [1 2 3]))", "[3 2 1]"},
	{"seq", "flatten", "(flatten [1 [2 [3 4]]])", "(1 2 3 4)"},
	{"seq", "distinct", "(vec (distinct [1 2 1 3 2]))", "[1 2 3]"},
	{"seq", "interleave", "(vec (interleave [1 2 3] [:a :b :c]))", "[1 :a 2 :b 3 :c]"},
	{"seq", "interpose", "(vec (interpose :x [1 2 3]))", "[1 :x 2 :x 3]"},
	{"seq", "partition", "(vec (map vec (partition 2 [1 2 3 4])))", "[[1 2] [3 4]]"},
	{"seq", "partition-all", "(vec (map vec (partition-all 2 [1 2 3 4 5])))", "[[1 2] [3 4] [5]]"},
	{"seq", "group-by", "(get (group-by even? [1 2 3 4]) true)", "[2 4]"},
	{"seq", "frequencies", "(get (frequencies [:a :b :a :c :a]) :a)", "3"},
	{"seq", "zipmap", "(:b (zipmap [:a :b :c] [1 2 3]))", "2"},
	{"seq", "range", "(vec (range 5))", "[0 1 2 3 4]"},
	{"seq", "range-start-end", "(vec (range 2 5))", "[2 3 4]"},
	{"seq", "repeat", "(vec (take 3 (repeat 42)))", "[42 42 42]"},
	{"seq", "repeatedly", "(count (take 5 (repeatedly #(rand-int 100))))", "5"},
	{"seq", "iterate", "(vec (take 5 (iterate inc 0)))", "[0 1 2 3 4]"},
	{"seq", "cycle", "(vec (take 6 (cycle [1 2 3])))", "[1 2 3 1 2 3]"},
	{"seq", "every?", "(every? even? [2 4 6])", "true"},
	{"seq", "some", "(some even? [1 2 3])", "true"},
	{"seq", "not-every?", "(not-every? even? [1 2 3])", "true"},
	{"seq", "not-any?", "(not-any? even? [1 3 5])", "true"},
	{"seq", "keep", "(vec (keep #(when (even? %) %) [1 2 3 4]))", "[2 4]"},
	{"seq", "map-indexed", "(vec (map-indexed vector [:a :b :c]))", "[[0 :a] [1 :b] [2 :c]]"},

	// --- Control flow ---
	{"control", "if-true", "(if true 1 2)", "1"},
	{"control", "if-false", "(if false 1 2)", "2"},
	{"control", "if-nil", "(if nil 1 2)", "2"},
	{"control", "when-true", "(when true 42)", "42"},
	{"control", "when-false", "(when false 42)", "nil"},
	{"control", "cond", "(cond (= 1 2) :a (= 1 1) :b :else :c)", ":b"},
	{"control", "case", "(case 2 1 :a 2 :b 3 :c)", ":b"},
	{"control", "and", "(and true true false)", "false"},
	{"control", "or", "(or false nil 42)", "42"},
	{"control", "not", "(not false)", "true"},
	{"control", "do", "(do 1 2 3)", "3"},

	// --- Functions ---
	{"fn", "defn", "(do (defn f [x] (* x x)) (f 5))", "25"},
	{"fn", "fn-literal", "((fn [x] (+ x 1)) 10)", "11"},
	{"fn", "apply", "(apply + [1 2 3])", "6"},
	{"fn", "comp", "((comp inc inc) 5)", "7"},
	{"fn", "partial", "((partial + 10) 5)", "15"},
	{"fn", "identity", "(identity 42)", "42"},
	{"fn", "constantly", "((constantly 42) :anything)", "42"},
	{"fn", "complement", "((complement even?) 3)", "true"},
	{"fn", "juxt", "((juxt + * min max) 3 4 6)", "[13 72 3 6]"},
	{"fn", "memoize", "(do (def mf (memoize (fn [x] (* x x)))) (mf 5))", "25"},

	// --- Let/binding ---
	{"binding", "let", "(let [x 1 y 2] (+ x y))", "3"},
	{"binding", "let-destructure-vec", "(let [[a b] [1 2]] (+ a b))", "3"},
	{"binding", "let-destructure-map", "(let [{:keys [a b]} {:a 1 :b 2}] (+ a b))", "3"},
	{"binding", "letfn", "(letfn [(f [x] (* x 2))] (f 21))", "42"},

	// --- Atoms ---
	{"atom", "atom-deref", "(deref (atom 42))", "42"},
	{"atom", "atom-swap", "(let [a (atom 0)] (swap! a inc) @a)", "1"},
	{"atom", "atom-reset", "(let [a (atom 0)] (reset! a 42) @a)", "42"},

	// --- Loop/recur ---
	{"loop", "basic", "(loop [i 0 s 0] (if (= i 5) s (recur (inc i) (+ s i))))", "10"},
	{"loop", "defn-recur", "(do (defn sum [n] (loop [i n acc 0] (if (zero? i) acc (recur (dec i) (+ acc i))))) (sum 100))", "5050"},

	// --- Type predicates ---
	{"type", "nil?", "(nil? nil)", "true"},
	{"type", "true?", "(true? true)", "true"},
	{"type", "false?", "(false? false)", "true"},
	{"type", "number?", "(number? 42)", "true"},
	{"type", "integer?", "(integer? 42)", "true"},
	{"type", "float?", "(float? 1.5)", "true"},
	{"type", "keyword?", "(keyword? :x)", "true"},
	{"type", "symbol?", "(symbol? 'x)", "true"},
	{"type", "fn?", "(fn? inc)", "true"},
	{"type", "coll?", "(coll? [1])", "true"},
	{"type", "seq?", "(seq? '(1))", "true"},
	{"type", "vector?", "(vector? [1])", "true"},
	{"type", "sequential?", "(sequential? [1])", "true"},
	{"type", "associative?", "(associative? {:a 1})", "true"},
	{"type", "counted?", "(counted? [1])", "true"},

	// --- Keywords/symbols ---
	{"kw-sym", "keyword", `(keyword "foo")`, ":foo"},
	{"kw-sym", "name", "(name :foo)", "foo"},
	{"kw-sym", "namespace-kw", "(namespace :foo/bar)", "foo"},
	{"kw-sym", "symbol", `(symbol "foo")`, "foo"},
	{"kw-sym", "gensym", "(symbol? (gensym))", "true"},

	// --- Regex ---
	{"regex", "re-find", `(re-find #"\d+" "abc123def")`, "123"},
	{"regex", "re-matches", `(re-matches #"\d+" "123")`, "123"},
	{"regex", "re-seq", `(vec (re-seq #"\d+" "a1b2c3"))`, `[1 2 3]`},

	// --- Transducers ---
	{"transducer", "transduce-map", "(transduce (map inc) + 0 [1 2 3])", "9"},
	{"transducer", "transduce-filter", "(transduce (filter even?) + 0 [1 2 3 4])", "6"},
	{"transducer", "transduce-take", "(transduce (take 2) conj [] [1 2 3 4])", "[1 2]"},
	{"transducer", "transduce-comp", "(transduce (comp (map inc) (filter even?)) + 0 [1 2 3 4])", "6"},

	// --- Misc ---
	{"misc", "pr-str", `(pr-str [1 "two" :three])`, `[1 "two" :three]`},
	{"misc", "hash-map", "(hash-map :a 1 :b 2)", "{:b 2, :a 1}"},
	{"misc", "set-fn", "(set [1 2 3 2 1])", "#{1 2 3}"},
	{"misc", "vec", "(vec '(1 2 3))", "[1 2 3]"},
	{"misc", "seq-fn", "(seq [1 2 3])", "(1 2 3)"},
	{"misc", "not-empty", "(not-empty [1])", "[1]"},
	{"misc", "not-empty-nil", "(not-empty [])", "nil"},
	{"misc", "rand-int", "(integer? (rand-int 100))", "true"},
}

func main() {
	jokerBin := "joker"
	outFile := ""

	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-joker":
			i++
			jokerBin = os.Args[i]
		case "-out":
			i++
			outFile = os.Args[i]
		}
	}

	var results []PResult
	pass, fail, errCount := 0, 0, 0

	for _, t := range parityTests {
		cmd := exec.Command(jokerBin, "-e", "(println "+t.Expr+")")
		out, err := cmd.CombinedOutput()
		got := strings.TrimSpace(string(out))

		r := PResult{
			Category: t.Category,
			Name:     t.Name,
			Expected: t.Expected,
			Got:      got,
		}

		if err != nil {
			r.Status = "error"
			r.Got = got
			errCount++
		} else if got == t.Expected {
			r.Status = "pass"
			pass++
		} else {
			r.Status = "fail"
			fail++
		}
		results = append(results, r)
	}

	total := len(results)
	fmt.Printf("Clojure parity: %d/%d pass, %d fail, %d error\n", pass, total, fail, errCount)

	// Print failures/errors
	for _, r := range results {
		if r.Status != "pass" {
			fmt.Printf("  %s %s/%s: expected=%q got=%q\n", r.Status, r.Category, r.Name, r.Expected, r.Got)
		}
	}

	// Write markdown divergence matrix
	if outFile != "" {
		var b strings.Builder
		b.WriteString("# Clojure parity divergence matrix\n\n")
		b.WriteString(fmt.Sprintf("_Generated: %s_\n\n", time.Now().Format("2006-01-02")))
		b.WriteString(fmt.Sprintf("**%d/%d pass** (%.0f%%), %d fail, %d error\n\n",
			pass, total, float64(pass)/float64(total)*100, fail, errCount))

		// Group by category
		cats := map[string][]PResult{}
		catOrder := []string{}
		for _, r := range results {
			if _, ok := cats[r.Category]; !ok {
				catOrder = append(catOrder, r.Category)
			}
			cats[r.Category] = append(cats[r.Category], r)
		}
		sort.Strings(catOrder)

		for _, cat := range catOrder {
			rs := cats[cat]
			catPass := 0
			for _, r := range rs {
				if r.Status == "pass" {
					catPass++
				}
			}
			b.WriteString(fmt.Sprintf("## %s (%d/%d)\n\n", cat, catPass, len(rs)))
			b.WriteString("| Test | Status | Expected | Got |\n")
			b.WriteString("|---|---|---|---|\n")
			for _, r := range rs {
				exp := r.Expected
				got := r.Got
				if len(exp) > 60 {
					exp = exp[:60] + "..."
				}
				if len(got) > 60 {
					got = got[:60] + "..."
				}
				b.WriteString(fmt.Sprintf("| %s | %s | `%s` | `%s` |\n", r.Name, r.Status, exp, got))
			}
			b.WriteString("\n")
		}

		os.WriteFile(outFile, []byte(b.String()), 0644)
		fmt.Printf("Report: %s\n", outFile)
	}
}
