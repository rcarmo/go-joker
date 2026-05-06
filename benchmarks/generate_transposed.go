//go:build ignore

package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

func main() {
	type Bench struct {
		name   string
		python float64
		goja   float64
		joker  float64
	}

	data := []Bench{
		{"n-body", 0.66, 4.75, 1.765},
		{"mandelbrot", 4.76, 39.0, 3.972},
		{"spectral", 24.48, 65.0, 17.35},
		{"binary-trees", 54.2, 172.0, 78.27},
		{"fannkuch", 4.94, 24.0, 33.70},
		{"fasta", 0.06, 0.60, 0.066},
		{"pidigits", 0.05, 0.15, 0.016},
		{"k-nucleotide", 0.03, 0.48, 0.251},
		{"rev-comp", 0.01, 0.13, 0.043},
		{"regex-redux", 0.09, 0.14, 0.083},
	}

	// Sort by Joker speed (fastest first)
	sort.Slice(data, func(i, j int) bool {
		return data[i].joker < data[j].joker
	})

	// Joker first row
	runtimes := []string{"Joker", "Python 3.13", "Goja (Go JS)"}

	cellW := 90
	cellH := 44
	headerH := 60
	labelW := 110
	w := labelW + len(data)*cellW + 20
	actualH := headerH + 3*cellH + 50

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
<style>
:root{color-scheme:light dark;--bg:#f8f9fc;--text:#1a1f36;--muted:#6b7394;--win:#dcfce7;--ok:#fef9c3;--slow:#fee2e2;--best:#16a34a;--border:#e5e7eb}
@media(prefers-color-scheme:dark){:root{--bg:#0d1117;--text:#e6edf3;--muted:#8b949e;--win:#064e3b;--ok:#422006;--slow:#450a0a;--best:#4ade80;--border:#30363d}}
svg{background:var(--bg);font-family:Inter,system-ui,sans-serif}
.title{fill:var(--text);font-size:15px;font-weight:700}
.subtitle{fill:var(--muted);font-size:10px}
.header{fill:var(--muted);font-size:9px;font-weight:600}
.rowlabel{fill:var(--text);font-size:11px;font-weight:600}
.val{fill:var(--text);font-size:10px}
.best-val{fill:var(--best);font-size:10px;font-weight:700}
.cell{stroke:var(--border);stroke-width:0.5}
</style>
`, w, actualH, w, actualH))

	b.WriteString(`<text class="title" x="10" y="20">CLBG Benchmarks — sorted by Joker speed (ms, lower is better)</text>`)
	b.WriteString(`<text class="subtitle" x="10" y="35">Green = fastest · Yellow = within 3× · Red = >3× slower</text>`)

	// Column headers
	for ci, d := range data {
		x := labelW + ci*cellW + cellW/2
		b.WriteString(fmt.Sprintf(`<text class="header" x="%d" y="%d" text-anchor="middle">%s</text>`, x, headerH-8, d.name))
	}

	// Rows: Joker, Python, Goja
	for ri, rtName := range runtimes {
		y := headerH + ri*cellH
		b.WriteString(fmt.Sprintf(`<text class="rowlabel" x="5" y="%d">%s</text>`, y+cellH/2+4, rtName))

		for ci, d := range data {
			x := labelW + ci*cellW
			// Order: joker, python, goja
			var vals [3]float64
			vals[0] = d.joker
			vals[1] = d.python
			vals[2] = d.goja
			v := vals[ri]
			minV := math.Min(math.Min(vals[0], vals[1]), vals[2])
			ratio := v / minV
			var fill string
			if ratio <= 1.1 {
				fill = "var(--win)"
			} else if ratio <= 3 {
				fill = "var(--ok)"
			} else {
				fill = "var(--slow)"
			}
			b.WriteString(fmt.Sprintf(`<rect class="cell" x="%d" y="%d" width="%d" height="%d" rx="4" fill="%s"/>`,
				x+2, y+2, cellW-4, cellH-4, fill))

			class := "val"
			if v == minV {
				class = "best-val"
			}
			label := fmt.Sprintf("%.3f", v)
			if v >= 1 {
				label = fmt.Sprintf("%.2f", v)
			}
			if v >= 10 {
				label = fmt.Sprintf("%.1f", v)
			}
			if v >= 100 {
				label = fmt.Sprintf("%.0f", v)
			}
			b.WriteString(fmt.Sprintf(`<text class="%s" x="%d" y="%d" text-anchor="middle">%s</text>`,
				class, x+cellW/2, y+cellH/2+4, label))
		}
	}

	b.WriteString(`</svg>`)
	os.WriteFile("benchmarks/benchmark-transposed.svg", []byte(b.String()), 0644)
	fmt.Println("wrote benchmarks/benchmark-transposed.svg")
}
