//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
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
	Series []series `json:"series"`
}

var lineRE = regexp.MustCompile(`^([a-zA-Z0-9_\-]+)\s+([0-9]+(?:\.[0-9]+)?)\s+ms/op`)

func canonical(name string) string {
	n := strings.ToLower(strings.ReplaceAll(name, "-", "_"))
	switch n {
	case "nbody_100steps":
		return "nbody"
	case "spectral_norm_50":
		return "spectral_norm"
	case "binary_trees_14":
		return "binary_trees"
	case "fannkuch_7":
		return "fannkuch"
	case "mandelbrot_200":
		return "mandelbrot"
	case "fasta_1000":
		return "fasta"
	case "pidigits_27":
		return "pidigits"
	case "arithmeticloop":
		return "arithmetic_loop"
	case "recursivefib":
		return "recursive_fib"
	case "tailrecursivesum":
		return "tail_recursive_sum"
	case "spectralnorm":
		return "spectral_norm"
	case "binarytrees":
		return "binary_trees"
	case "reversecomplement":
		return "reverse_complement"
	case "regexredux":
		return "regex_redux"
	case "mapupdateloop":
		return "map_update_loop"
	case "wordfrequency":
		return "word_frequency"
	default:
		return n
	}
}

func parseRuntimeFile(path string) (map[string]float64, error) {
	m := map[string]float64{}
	if path == "" {
		return nil, fmt.Errorf("runtime output path is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not close %s: %v\n", path, err)
		}
	}()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "# SKIPPED") || line == "" {
			continue
		}
		parts := lineRE.FindStringSubmatch(line)
		if len(parts) != 3 {
			continue
		}
		var v float64
		if _, err := fmt.Sscanf(parts[2], "%f", &v); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not parse runtime %q in %s: %v\n", parts[2], path, err)
			continue
		}
		m[canonical(parts[1])] = v
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

func format(v float64, ok bool) string {
	if !ok {
		return "-"
	}
	if v < 1 {
		return fmt.Sprintf("%.3f", v)
	}
	if v < 10 {
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func main() {
	historyPath := flag.String("history", "benchmarks/benchmark-history.json", "Path to benchmark-history.json")
	pythonPath := flag.String("python", "", "Path to python runtime output")
	bunPath := flag.String("bun", "", "Path to bun runtime output")
	gojaPath := flag.String("goja", "", "Path to goja runtime output")
	letgoPath := flag.String("letgo", "", "Path to let-go runtime output")
	outPath := flag.String("out", "", "Output markdown file")
	flag.Parse()

	if *outPath == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}

	data, err := os.ReadFile(*historyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read history: %v\n", err)
		os.Exit(1)
	}
	var h history
	if err := json.Unmarshal(data, &h); err != nil {
		fmt.Fprintf(os.Stderr, "parse history: %v\n", err)
		os.Exit(1)
	}

	joker := map[string]float64{}
	for _, s := range h.Series {
		if s.ID == "current" {
			for k, v := range s.Benchmarks {
				joker[k] = v.MSPerOp
			}
		}
	}

	if len(joker) == 0 {
		fmt.Fprintln(os.Stderr, "benchmark history has no current Joker series")
		os.Exit(1)
	}

	python, err := parseRuntimeFile(*pythonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse python output: %v\n", err)
		os.Exit(1)
	}
	bun, err := parseRuntimeFile(*bunPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse bun output: %v\n", err)
		os.Exit(1)
	}
	goja, err := parseRuntimeFile(*gojaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse goja output: %v\n", err)
		os.Exit(1)
	}
	letgo, err := parseRuntimeFile(*letgoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse let-go output: %v\n", err)
		os.Exit(1)
	}

	keysMap := map[string]struct{}{}
	for k := range joker {
		keysMap[k] = struct{}{}
	}
	for k := range python {
		keysMap[k] = struct{}{}
	}
	for k := range bun {
		keysMap[k] = struct{}{}
	}
	for k := range goja {
		keysMap[k] = struct{}{}
	}
	for k := range letgo {
		keysMap[k] = struct{}{}
	}

	preferred := []string{
		"arithmetic_loop", "recursive_fib", "tail_recursive_sum", "map_update_loop", "word_frequency", "nbody", "spectral_norm", "binary_trees", "fannkuch", "mandelbrot", "fasta", "knucleotide", "reverse_complement", "regex_redux", "pidigits",
	}
	keys := make([]string, 0, len(keysMap))
	seen := map[string]bool{}
	for _, k := range preferred {
		if _, ok := keysMap[k]; ok {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	for k := range keysMap {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys[len(preferred):])

	var b strings.Builder
	b.WriteString("# Direct runtime comparison\n\n")
	b.WriteString("Generated from `benchmark-history.json` (Joker) and cross-language runtime scripts. Values are ms/op; lower is better.\n\n")
	b.WriteString("| Benchmark | Joker | Python | Bun/corecollections.Node | Goja | let-go | Winner |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---|\n")

	for _, k := range keys {
		j, jok := joker[k]
		p, pok := python[k]
		bu, buok := bun[k]
		g, gok := goja[k]
		l, lok := letgo[k]

		winner := "-"
		minV := 0.0
		pick := func(label string, ok bool, v float64) {
			if !ok {
				return
			}
			if winner == "-" || v < minV {
				winner = label
				minV = v
			}
		}
		pick("Joker", jok, j)
		pick("Python", pok, p)
		pick("Bun/corecollections.Node", buok, bu)
		pick("Goja", gok, g)
		pick("let-go", lok, l)

		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s |\n",
			strings.ReplaceAll(k, "_", "-"),
			format(j, jok),
			format(p, pok),
			format(bu, buok),
			format(g, gok),
			format(l, lok),
			winner,
		)
	}

	if err := os.WriteFile(*outPath, []byte(b.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("wrote", *outPath)
}
