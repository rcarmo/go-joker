//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type benchValue struct {
	MSPerOp float64 `json:"ms_per_op"`
}

type benchSeries struct {
	ID         string                `json:"id"`
	Benchmarks map[string]benchValue `json:"benchmarks"`
}

type benchHistory struct {
	Metadata struct {
		Updated       string   `json:"updated"`
		NonComparable []string `json:"non_comparable_baseline"`
	} `json:"metadata"`
	Series []benchSeries `json:"series"`
}

type Row struct {
	name   string
	before float64
	after  float64
}

var speedupOrder = []string{
	"arithmetic_loop",
	"recursive_fib",
	"mandelbrot",
	"nbody",
	"spectral_norm",
	"binary_trees",
	"fannkuch",
	"fasta",
	"pidigits",
	"knucleotide",
	"reverse_complement",
	"regex_redux",
	"word_frequency",
}

func main() {
	history := readHistory("benchmarks/benchmark-history.json")
	baseline := findSeries(history, "baseline-stable")
	current := findSeries(history, "current")
	data := make([]Row, 0, len(speedupOrder))
	for _, key := range speedupOrder {
		skip := false
		for _, excluded := range history.Metadata.NonComparable {
			if key == excluded {
				skip = true
			}
		}
		if skip {
			continue
		}
		before, ok := baseline.Benchmarks[key]
		if !ok || before.MSPerOp <= 0 {
			panic(fmt.Sprintf("missing positive baseline benchmark value for %s", key))
		}
		after, ok := current.Benchmarks[key]
		if !ok || after.MSPerOp <= 0 {
			panic(fmt.Sprintf("missing positive current benchmark value for %s", key))
		}
		data = append(data, Row{name: displayName(key), before: before.MSPerOp, after: after.MSPerOp})
	}

	sort.Slice(data, func(i, j int) bool {
		return (data[i].before / data[i].after) > (data[j].before / data[j].after)
	})

	rowH := 38
	headerH := 60
	w := 750
	h := headerH + len(data)*rowH + 20
	barMaxW := 320

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
<style>
:root{color-scheme:light dark;--bg:#f8f9fc;--text:#1a1f36;--muted:#6b7394;--bar1:#e2e8f0;--bar2:#22c55e;--border:#e5e7eb}
@media(prefers-color-scheme:dark){:root{--bg:#0d1117;--text:#e6edf3;--muted:#8b949e;--bar1:#1e293b;--bar2:#4ade80;--border:#30363d}}
svg{background:var(--bg);font-family:Inter,system-ui,sans-serif}
.title{fill:var(--text);font-size:16px;font-weight:700}
.subtitle{fill:var(--muted);font-size:10px}
.name{fill:var(--text);font-size:11px;font-weight:500}
.before{fill:var(--muted);font-size:10px}
.after{fill:var(--text);font-size:10px;font-weight:600}
.delta{fill:var(--text);font-size:11px;font-weight:700}
.bar-bg{fill:var(--bar1);rx:3}
.bar-fg{fill:var(--bar2);rx:3}
.arrow{fill:var(--muted);font-size:10px}
</style>
`, w, h, w, h))

	b.WriteString(`<text class="title" x="15" y="22">Speedup vs Original Joker</text>`)
	b.WriteString(`<text class="subtitle" x="15" y="38">Historical baseline → best-Joker 2026-09-06 (ms) · incompatible pidigits excluded</text>`)

	b.WriteString(fmt.Sprintf(`<text class="before" x="130" y="%d">Before</text>`, headerH-8))
	b.WriteString(fmt.Sprintf(`<text class="before" x="195" y="%d">After</text>`, headerH-8))
	b.WriteString(fmt.Sprintf(`<text class="before" x="380" y="%d">Speedup</text>`, headerH-8))

	maxLog := math.Log10(900)

	for i, d := range data {
		y := headerH + i*rowH
		speedup := d.before / d.after

		b.WriteString(fmt.Sprintf(`<text class="name" x="15" y="%d">%s</text>`, y+rowH/2+4, d.name))
		b.WriteString(fmt.Sprintf(`<text class="before" x="130" y="%d">%s</text>`, y+rowH/2+4, fmtMs(d.before)))
		b.WriteString(fmt.Sprintf(`<text class="arrow" x="175" y="%d">→</text>`, y+rowH/2+4))
		b.WriteString(fmt.Sprintf(`<text class="after" x="195" y="%d">%s</text>`, y+rowH/2+4, fmtMs(d.after)))

		barX := 250
		barY := y + 10
		barH := rowH - 20
		b.WriteString(fmt.Sprintf(`<rect class="bar-bg" x="%d" y="%d" width="%d" height="%d"/>`, barX, barY, barMaxW, barH))

		barW := int(math.Log10(speedup) / maxLog * float64(barMaxW))
		if barW < 4 {
			barW = 4
		}
		if barW > barMaxW {
			barW = barMaxW
		}
		b.WriteString(fmt.Sprintf(`<rect class="bar-fg" x="%d" y="%d" width="%d" height="%d"/>`, barX, barY, barW, barH))

		deltaStr := fmt.Sprintf("%.0f×", speedup)
		if speedup < 10 {
			deltaStr = fmt.Sprintf("%.1f×", speedup)
		}
		b.WriteString(fmt.Sprintf(`<text class="delta" x="%d" y="%d">%s</text>`, barX+barMaxW+10, y+rowH/2+4, deltaStr))
	}

	b.WriteString(`</svg>`)
	if err := os.WriteFile("benchmarks/benchmark-speedup.svg", []byte(b.String()), 0644); err != nil {
		panic(err)
	}
	fmt.Println("wrote benchmark-speedup.svg")
}

func readHistory(path string) benchHistory {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var h benchHistory
	if err := json.Unmarshal(data, &h); err != nil {
		panic(err)
	}
	return h
}

func findSeries(h benchHistory, id string) benchSeries {
	for _, s := range h.Series {
		if s.ID == id {
			return s
		}
	}
	panic("missing benchmark series: " + id)
}

func displayName(key string) string {
	switch key {
	case "arithmetic_loop":
		return "arith-loop"
	case "recursive_fib":
		return "rec-fib"
	case "nbody":
		return "n-body"
	case "reverse_complement":
		return "rev-complement"
	case "regex_redux":
		return "regex-redux"
	default:
		return strings.ReplaceAll(key, "_", "-")
	}
}

func fmtMs(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("%.0fms", v)
	}
	if v >= 10 {
		return fmt.Sprintf("%.1fms", v)
	}
	if v >= 1 {
		return fmt.Sprintf("%.2fms", v)
	}
	return fmt.Sprintf("%.3fms", v)
}
