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

type history struct {
	Series []struct {
		ID         string `json:"id"`
		Benchmarks map[string]struct {
			MS float64 `json:"ms_per_op"`
		} `json:"benchmarks"`
	} `json:"series"`
	CrossLanguage map[string]map[string]float64 `json:"cross_language"`
}

type bench struct {
	key   string
	name  string
	joker float64
	py    float64
	bun   float64
	goja  float64
	letgo float64
}

func main() {
	var h history
	data, err := os.ReadFile("benchmarks/benchmark-history.json")
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(data, &h); err != nil {
		panic(err)
	}
	current := findSeries(h, "current")
	names := map[string]string{
		"nbody": "n-body", "mandelbrot": "mandelbrot", "spectral_norm": "spectral", "binary_trees": "binary-trees",
		"fannkuch": "fannkuch", "fasta": "fasta", "pidigits": "pidigits", "knucleotide": "k-nucleotide",
		"reverse_complement": "rev-comp", "regex_redux": "regex-redux", "arithmetic_loop": "arith-loop", "recursive_fib": "rec-fib",
		"tail_recursive_sum": "tail-sum", "map_update_loop": "map-update", "word_frequency": "word-freq",
	}
	order := []string{"nbody", "mandelbrot", "spectral_norm", "binary_trees", "fannkuch", "fasta", "pidigits", "knucleotide", "reverse_complement", "regex_redux", "arithmetic_loop", "recursive_fib", "tail_recursive_sum", "map_update_loop", "word_frequency"}
	rows := make([]bench, 0, len(order))
	for _, k := range order {
		rows = append(rows, bench{
			key:   k,
			name:  names[k],
			joker: requiredCurrent(current, k),
			py:    requiredCross(h, "python_313", k),
			bun:   requiredCross(h, "bun_jsc", k),
			goja:  requiredCross(h, "goja", k),
			letgo: requiredCross(h, "letgo", k),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].joker < rows[j].joker })

	runtimes := []string{"Joker", "Python 3.14", "Bun/JSC", "Goja", "let-go"}
	cellW, cellH, headerH, labelW := 82, 44, 60, 110
	w := labelW + len(rows)*cellW + 20
	hgt := headerH + len(runtimes)*cellH + 50

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
<style>
:root{color-scheme:light dark;--bg:#f8f9fc;--text:#1a1f36;--muted:#6b7394;--win:#dcfce7;--ok:#fef9c3;--slow:#fee2e2;--best:#16a34a;--border:#e5e7eb}
@media(prefers-color-scheme:dark){:root{--bg:#0d1117;--text:#e6edf3;--muted:#8b949e;--win:#064e3b;--ok:#422006;--slow:#450a0a;--best:#4ade80;--border:#30363d}}
svg{background:var(--bg);font-family:Inter,system-ui,sans-serif}.title{fill:var(--text);font-size:15px;font-weight:700}.subtitle{fill:var(--muted);font-size:10px}.header{fill:var(--muted);font-size:9px;font-weight:600}.rowlabel{fill:var(--text);font-size:11px;font-weight:600}.val{fill:var(--text);font-size:10px}.best-val{fill:var(--best);font-size:10px;font-weight:700}.cell{stroke:var(--border);stroke-width:0.5}
</style>
`, w, hgt, w, hgt))
	b.WriteString(`<text class="title" x="10" y="20">CLBG + micro comparisons — ms/op, lower is better</text>`)
	b.WriteString(`<text class="subtitle" x="10" y="35">Green = fastest · Yellow = within 3× · Red = >3× slower · includes let-go v1.7.4</text>`)
	for ci, d := range rows {
		x := labelW + ci*cellW + cellW/2
		b.WriteString(fmt.Sprintf(`<text class="header" x="%d" y="%d" text-anchor="middle">%s</text>`, x, headerH-8, d.name))
	}
	for ri, rt := range runtimes {
		y := headerH + ri*cellH
		b.WriteString(fmt.Sprintf(`<text class="rowlabel" x="5" y="%d">%s</text>`, y+cellH/2+4, rt))
		for ci, d := range rows {
			vals := []float64{d.joker, d.py, d.bun, d.goja, d.letgo}
			v := vals[ri]
			minV := vals[0]
			for _, vv := range vals[1:] {
				if vv < minV {
					minV = vv
				}
			}
			ratio := v / minV
			fill := "var(--slow)"
			if ratio <= 1.1 {
				fill = "var(--win)"
			} else if ratio <= 3 {
				fill = "var(--ok)"
			}
			x := labelW + ci*cellW
			b.WriteString(fmt.Sprintf(`<rect class="cell" x="%d" y="%d" width="%d" height="%d" rx="4" fill="%s"/>`, x+2, y+2, cellW-4, cellH-4, fill))
			class := "val"
			if v == minV {
				class = "best-val"
			}
			b.WriteString(fmt.Sprintf(`<text class="%s" x="%d" y="%d" text-anchor="middle">%s</text>`, class, x+cellW/2, y+cellH/2+4, fmtMs(v)))
		}
	}
	b.WriteString(`</svg>`)
	if err := os.WriteFile("benchmarks/benchmark-transposed.svg", []byte(b.String()), 0644); err != nil {
		panic(err)
	}
	fmt.Println("wrote benchmarks/benchmark-transposed.svg")
}

func findSeries(h history, id string) map[string]struct {
	MS float64 `json:"ms_per_op"`
} {
	for _, s := range h.Series {
		if s.ID == id {
			return s.Benchmarks
		}
	}
	panic("missing benchmark series: " + id)
}

func requiredCurrent(current map[string]struct {
	MS float64 `json:"ms_per_op"`
}, key string) float64 {
	v, ok := current[key]
	if !ok || v.MS <= 0 {
		panic(fmt.Sprintf("missing positive current benchmark value for %s", key))
	}
	return v.MS
}

func requiredCross(h history, runtime string, key string) float64 {
	vals, ok := h.CrossLanguage[runtime]
	if !ok {
		panic("missing cross-language runtime: " + runtime)
	}
	v, ok := vals[key]
	if !ok || v <= 0 {
		panic(fmt.Sprintf("missing positive %s benchmark value for %s", runtime, key))
	}
	return v
}

func fmtMs(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "-"
	}
	if v >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	if v >= 10 {
		return fmt.Sprintf("%.1f", v)
	}
	if v >= 1 {
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("%.3f", v)
}
