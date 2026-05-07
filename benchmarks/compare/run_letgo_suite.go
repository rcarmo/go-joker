//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type runtimeSpec struct {
	Name string
	Bin  string
	Kind string // clj or direct
	Skip map[string]bool
}

type benchStat struct {
	MeanMS   float64 `json:"mean_ms"`
	StddevMS float64 `json:"stddev_ms"`
	Runs     int     `json:"runs"`
	Skipped  bool    `json:"skipped,omitempty"`
	Reason   string  `json:"reason,omitempty"`
}

type output struct {
	Warmup     int                             `json:"warmup"`
	Runs       int                             `json:"runs"`
	BenchDir   string                          `json:"bench_dir"`
	Runtimes   []string                        `json:"runtimes"`
	Benchmarks []string                        `json:"benchmarks"`
	Results    map[string]map[string]benchStat `json:"results"`
}

var benchmarkOrder = []string{
	"fib",
	"loop-recur",
	"map-filter",
	"persistent-map",
	"reduce",
	"tak",
	"transducers",
}

func lookPath(candidates ...string) string {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if strings.Contains(c, "/") {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c
			}
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

func argsFor(kind, benchPath string) []string {
	if kind == "clj" {
		return []string{"-M", "-e", fmt.Sprintf("(load-file %q)", benchPath)}
	}
	return []string{benchPath}
}

func runOnce(bin, kind, benchPath string) (time.Duration, error) {
	args := argsFor(kind, benchPath)
	cmd := exec.Command(bin, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)
	if err != nil {
		msg := strings.TrimSpace(out.String())
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		if msg != "" {
			return dur, fmt.Errorf("%w: %s", err, msg)
		}
		return dur, err
	}
	return dur, nil
}

func meanStddev(vals []float64) (float64, float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean := sum / float64(len(vals))
	if len(vals) == 1 {
		return mean, 0
	}
	var ss float64
	for _, v := range vals {
		d := v - mean
		ss += d * d
	}
	return mean, math.Sqrt(ss / float64(len(vals)))
}

func fmtMS(v float64, ok bool) string {
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
	benchDir := flag.String("bench-dir", "benchmarks/compare/letgo_suite", "Directory with benchmark .clj files")
	outMD := flag.String("out", "", "Output markdown file")
	outJSON := flag.String("json", "", "Optional JSON output file")
	warmup := flag.Int("warmup", 3, "Warmup runs per benchmark/runtime")
	runs := flag.Int("runs", 10, "Timed runs per benchmark/runtime")
	letgoBin := flag.String("letgo-bin", os.Getenv("LETGO_BIN"), "Path to let-go binary (or leave empty for auto-detect)")
	gojokerBin := flag.String("gojoker-bin", os.Getenv("GOJOKER_BIN"), "Path to go-joker binary (or leave empty for auto-detect)")
	jokerBin := flag.String("joker-bin", os.Getenv("JOKER_BIN"), "Path to upstream joker binary (optional)")
	bbBin := flag.String("bb-bin", os.Getenv("BB_BIN"), "Path to babashka binary (optional)")
	cljBin := flag.String("clj-bin", os.Getenv("CLJ_BIN"), "Path to clj binary (optional)")
	flag.Parse()

	if *outMD == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}

	*letgoBin = lookPath(*letgoBin, "lg", "let-go")
	*gojokerBin = lookPath(*gojokerBin, "go-joker")
	*jokerBin = lookPath(*jokerBin, "joker")
	*bbBin = lookPath(*bbBin, "bb")
	*cljBin = lookPath(*cljBin, "clj")

	runtimes := []runtimeSpec{}
	if *letgoBin != "" {
		runtimes = append(runtimes, runtimeSpec{Name: "let-go", Bin: *letgoBin, Kind: "direct", Skip: map[string]bool{}})
	}
	if *gojokerBin != "" {
		runtimes = append(runtimes, runtimeSpec{Name: "go-joker", Bin: *gojokerBin, Kind: "direct", Skip: map[string]bool{}})
	}
	if *jokerBin != "" {
		runtimes = append(runtimes, runtimeSpec{Name: "joker", Bin: *jokerBin, Kind: "direct", Skip: map[string]bool{}})
	}
	if *bbBin != "" {
		runtimes = append(runtimes, runtimeSpec{Name: "babashka", Bin: *bbBin, Kind: "direct", Skip: map[string]bool{}})
	}
	if *cljBin != "" {
		runtimes = append(runtimes, runtimeSpec{Name: "clojure-jvm", Bin: *cljBin, Kind: "clj", Skip: map[string]bool{}})
	}

	if len(runtimes) == 0 {
		fmt.Fprintln(os.Stderr, "no supported runtime binaries found (let-go/go-joker/joker/bb/clj)")
		os.Exit(1)
	}

	benchmarks := []string{}
	for _, b := range benchmarkOrder {
		p := filepath.Join(*benchDir, b+".clj")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			benchmarks = append(benchmarks, b)
		}
	}
	if len(benchmarks) == 0 {
		fmt.Fprintf(os.Stderr, "no benchmark files found in %s\n", *benchDir)
		os.Exit(1)
	}

	results := map[string]map[string]benchStat{}
	for _, b := range benchmarks {
		results[b] = map[string]benchStat{}
		benchPath := filepath.Join(*benchDir, b+".clj")
		for _, rt := range runtimes {
			if rt.Skip[b] {
				results[b][rt.Name] = benchStat{Skipped: true, Reason: "policy skip"}
				continue
			}

			failed := false
			for i := 0; i < *warmup; i++ {
				if _, err := runOnce(rt.Bin, rt.Kind, benchPath); err != nil {
					results[b][rt.Name] = benchStat{Skipped: true, Reason: fmt.Sprintf("warmup failed: %v", err)}
					failed = true
					break
				}
			}
			if failed {
				continue
			}

			times := make([]float64, 0, *runs)
			for i := 0; i < *runs; i++ {
				d, err := runOnce(rt.Bin, rt.Kind, benchPath)
				if err != nil {
					results[b][rt.Name] = benchStat{Skipped: true, Reason: fmt.Sprintf("timed run failed: %v", err)}
					failed = true
					break
				}
				times = append(times, float64(d)/float64(time.Millisecond))
			}
			if failed {
				continue
			}
			m, s := meanStddev(times)
			results[b][rt.Name] = benchStat{MeanMS: m, StddevMS: s, Runs: len(times)}
		}
	}

	var md strings.Builder
	md.WriteString("# let-go benchmark suite comparison\n\n")
	md.WriteString("Benchmarks mirrored from `nooga/let-go/benchmark` (`fib`, `loop-recur`, `map-filter`, `persistent-map`, `reduce`, `tak`, `transducers`).\n\n")
	md.WriteString(fmt.Sprintf("Warmup: %d, timed runs: %d. Values are ms/op (lower is better).\n\n", *warmup, *runs))

	md.WriteString("| Benchmark |")
	for _, rt := range runtimes {
		md.WriteString(" " + rt.Name + " |")
	}
	md.WriteString(" Winner |\n")
	md.WriteString("|---|")
	for range runtimes {
		md.WriteString("---:|")
	}
	md.WriteString("---|\n")

	for _, b := range benchmarks {
		md.WriteString("| " + b + " |")
		winner := "-"
		minVal := 0.0
		for _, rt := range runtimes {
			st, ok := results[b][rt.Name]
			if !ok || st.Skipped {
				md.WriteString(" - |")
				continue
			}
			md.WriteString(" " + fmtMS(st.MeanMS, true) + " |")
			if winner == "-" || st.MeanMS < minVal {
				winner = rt.Name
				minVal = st.MeanMS
			}
		}
		md.WriteString(" " + winner + " |\n")
	}

	md.WriteString("\n## Runtime notes\n\n")
	for _, rt := range runtimes {
		skips := []string{}
		for _, b := range benchmarks {
			if st, ok := results[b][rt.Name]; ok && st.Skipped {
				skips = append(skips, fmt.Sprintf("%s (%s)", b, st.Reason))
			}
		}
		sort.Strings(skips)
		if len(skips) == 0 {
			md.WriteString(fmt.Sprintf("- %s: all benchmarks ran.\n", rt.Name))
		} else {
			md.WriteString(fmt.Sprintf("- %s: %s\n", rt.Name, strings.Join(skips, "; ")))
		}
	}

	if err := os.WriteFile(*outMD, []byte(md.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write markdown: %v\n", err)
		os.Exit(1)
	}

	if *outJSON != "" {
		rtNames := make([]string, 0, len(runtimes))
		for _, rt := range runtimes {
			rtNames = append(rtNames, rt.Name)
		}
		payload := output{
			Warmup:     *warmup,
			Runs:       *runs,
			BenchDir:   *benchDir,
			Runtimes:   rtNames,
			Benchmarks: benchmarks,
			Results:    results,
		}
		buf, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "encode json: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*outJSON, buf, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write json: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("wrote", *outMD)
	if *outJSON != "" {
		fmt.Println("wrote", *outJSON)
	}
}
