//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
)

type benchmark struct {
	MSPerOp float64 `json:"ms_per_op"`
}
type series struct {
	ID         string               `json:"id"`
	Benchmarks map[string]benchmark `json:"benchmarks"`
}
type history struct {
	Series        []series                      `json:"series"`
	CrossLanguage map[string]map[string]float64 `json:"cross_language"`
}

var rowRE = regexp.MustCompile(`^\| ([^|]+) \| ([^|]+)ms \| ([^|]+)ms \| ([^|]+)ms \| ([^|]+)ms \| ([^|]+)ms \| ([^|]+) \|$`)

var ordered = []string{"arithmetic_loop", "recursive_fib", "tail_recursive_sum", "map_update_loop", "word_frequency", "nbody", "spectral_norm", "binary_trees", "fannkuch", "mandelbrot", "fasta", "knucleotide", "reverse_complement", "regex_redux", "pidigits"}

func display(k string) string { return strings.ReplaceAll(k, "_", "-") }

func fmtCell(v float64) string {
	if v < 1 {
		return fmt.Sprintf("%.3f", v)
	}
	if v < 10 {
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func closeEnough(got string, want float64) bool {
	var v float64
	if _, err := fmt.Sscanf(strings.TrimSpace(got), "%f", &v); err != nil {
		return false
	}
	return math.Abs(v-want) < 0.0005 || strings.TrimSpace(got) == fmtCell(want)
}

func winner(vals map[string]float64) string {
	labels := []string{"Joker", "Python", "Bun/JSC", "Goja", "let-go"}
	minLabel := ""
	min := 0.0
	for _, label := range labels {
		v := vals[label]
		if minLabel == "" || v < min {
			minLabel, min = label, v
		}
	}
	return minLabel
}

func main() {
	data, err := os.ReadFile("benchmarks/benchmark-history.json")
	if err != nil {
		panic(err)
	}
	var h history
	if err := json.Unmarshal(data, &h); err != nil {
		panic(err)
	}
	current := map[string]benchmark{}
	for _, s := range h.Series {
		if s.ID == "current" {
			current = s.Benchmarks
		}
	}
	readme, err := os.ReadFile("benchmarks/README.md")
	if err != nil {
		panic(err)
	}
	rows := map[string][]string{}
	for _, line := range strings.Split(string(readme), "\n") {
		m := rowRE.FindStringSubmatch(line)
		if len(m) == 8 {
			rows[strings.ReplaceAll(m[1], "-", "_")] = m[2:]
		}
	}
	for _, k := range ordered {
		row, ok := rows[k]
		if !ok {
			fmt.Fprintf(os.Stderr, "missing benchmark README row for %s\n", display(k))
			os.Exit(1)
		}
		vals := map[string]float64{
			"Joker":   current[k].MSPerOp,
			"Python":  h.CrossLanguage["python_313"][k],
			"Bun/JSC": h.CrossLanguage["bun_jsc"][k],
			"Goja":    h.CrossLanguage["goja"][k],
			"let-go":  h.CrossLanguage["letgo"][k],
		}
		checks := []struct {
			label, got string
			want       float64
		}{{"Joker", row[0], vals["Joker"]}, {"Python", row[1], vals["Python"]}, {"Bun/JSC", row[2], vals["Bun/JSC"]}, {"Goja", row[3], vals["Goja"]}, {"let-go", row[4], vals["let-go"]}}
		for _, c := range checks {
			if !closeEnough(c.got, c.want) {
				fmt.Fprintf(os.Stderr, "%s %s README value = %s, want %s\n", display(k), c.label, c.got, fmtCell(c.want))
				os.Exit(1)
			}
		}
		if got, want := strings.TrimSpace(row[5]), winner(vals); got != want {
			fmt.Fprintf(os.Stderr, "%s winner = %s, want %s\n", display(k), got, want)
			os.Exit(1)
		}
	}
	fmt.Println("benchmark README table matches benchmark-history.json")
}
