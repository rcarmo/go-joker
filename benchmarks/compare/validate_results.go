//go:build ignore

package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var resultRE = regexp.MustCompile(`^([a-zA-Z0-9_\-]+)\s+[0-9]+(?:\.[0-9]+)?\s+ms/op\s+\(result:\s+([^)]*)\)$`)

var expectedFloatResults = map[string]float64{
	"nbody":         -0.16926665164096838,
	"spectral_norm": 1.2741938369830932,
}

var expectedResults = map[string]string{
	"arithmetic_loop":    "500001",
	"recursive_fib":      "139104",
	"tail_recursive_sum": "5000050000",
	"nbody":              "-0.169267",
	"spectral_norm":      "1.274193837",
	"binary_trees":       "358401",
	"fannkuch":           "16228",
	"mandelbrot":         "633",
	"fasta":              "150034",
	"knucleotide":        "27",
	"reverse_complement": "196",
	"map_update_loop":    "938",
	"word_frequency":     "1000",
	"regex_redux":        "8",
	"pidigits":           "129",
}

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

func validateFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	seen := map[string]bool{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "# SKIPPED") {
			continue
		}
		m := resultRE.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}
		name := canonical(m[1])
		want, ok := expectedResults[name]
		if !ok {
			return fmt.Errorf("%s: no expected result for %s", path, name)
		}
		got := strings.TrimSuffix(strings.TrimSpace(m[2]), "N")
		if wantFloat, ok := expectedFloatResults[name]; ok {
			gotFloat, err := strconv.ParseFloat(got, 64)
			if err != nil || math.Abs(gotFloat-wantFloat) > 5e-7 {
				return fmt.Errorf("%s: %s result = %s, want %.17g ± 5e-7", path, name, got, wantFloat)
			}
		} else if got != want {
			return fmt.Errorf("%s: %s result = %s, want %s", path, name, got, want)
		}
		seen[name] = true
	}
	if err := s.Err(); err != nil {
		return err
	}
	for name := range expectedResults {
		if !seen[name] {
			return fmt.Errorf("%s: missing result for %s", path, name)
		}
	}
	return nil
}

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: validate_results.go runtime-output.txt...")
		os.Exit(2)
	}
	for _, path := range flag.Args() {
		if err := validateFile(path); err != nil {
			fmt.Fprintln(os.Stderr, "benchmark result validation failed:", err)
			os.Exit(1)
		}
		fmt.Println("validated", path)
	}
}
