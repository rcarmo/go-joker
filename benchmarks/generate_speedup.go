//go:build ignore

package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type Row struct {
	name   string
	before float64
	after  float64
}

func main() {
	data := []Row{
		{"arith-loop", 189.78, 0.237},
		{"rec-fib", 546.02, 0.959},
		{"mandelbrot", 159.0, 3.972},
		{"n-body", 34.2, 1.765},
		{"spectral-norm", 70.0, 17.35},
		{"binary-trees", 528.0, 78.27},
		{"fannkuch", 94.1, 33.70},
		{"fasta", 0.22, 0.066},
		{"pidigits", 0.10, 0.016},
		{"knucleotide", 0.41, 0.251},
		{"rev-complement", 0.082, 0.043},
		{"regex-redux", 0.12, 0.083},
		{"word-frequency", 279.92, 13.6},
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
	b.WriteString(`<text class="subtitle" x="15" y="38">Before → After (ms) · bar shows improvement factor (log scale)</text>`)

	// Column headers
	b.WriteString(fmt.Sprintf(`<text class="before" x="130" y="%d">Before</text>`, headerH-8))
	b.WriteString(fmt.Sprintf(`<text class="before" x="195" y="%d">After</text>`, headerH-8))
	b.WriteString(fmt.Sprintf(`<text class="before" x="380" y="%d">Speedup</text>`, headerH-8))

	maxLog := math.Log10(900)

	for i, d := range data {
		y := headerH + i*rowH
		speedup := d.before / d.after

		// Name
		b.WriteString(fmt.Sprintf(`<text class="name" x="15" y="%d">%s</text>`, y+rowH/2+4, d.name))

		// Before value
		beforeStr := fmtMs(d.before)
		b.WriteString(fmt.Sprintf(`<text class="before" x="130" y="%d">%s</text>`, y+rowH/2+4, beforeStr))

		// Arrow
		b.WriteString(fmt.Sprintf(`<text class="arrow" x="175" y="%d">→</text>`, y+rowH/2+4))

		// After value
		afterStr := fmtMs(d.after)
		b.WriteString(fmt.Sprintf(`<text class="after" x="195" y="%d">%s</text>`, y+rowH/2+4, afterStr))

		// Bar background
		barX := 250
		barY := y + 10
		barH := rowH - 20
		b.WriteString(fmt.Sprintf(`<rect class="bar-bg" x="%d" y="%d" width="%d" height="%d"/>`, barX, barY, barMaxW, barH))

		// Bar foreground (log scale)
		logSpeedup := math.Log10(speedup)
		barW := int(logSpeedup / maxLog * float64(barMaxW))
		if barW < 4 {
			barW = 4
		}
		if barW > barMaxW {
			barW = barMaxW
		}
		b.WriteString(fmt.Sprintf(`<rect class="bar-fg" x="%d" y="%d" width="%d" height="%d"/>`, barX, barY, barW, barH))

		// Speedup label
		deltaStr := fmt.Sprintf("%.0f×", speedup)
		if speedup < 10 {
			deltaStr = fmt.Sprintf("%.1f×", speedup)
		}
		b.WriteString(fmt.Sprintf(`<text class="delta" x="%d" y="%d">%s</text>`, barX+barMaxW+10, y+rowH/2+4, deltaStr))
	}

	b.WriteString(`</svg>`)
	os.WriteFile("/workspace/projects/go-joker/benchmarks/benchmark-speedup.svg", []byte(b.String()), 0644)
	fmt.Println("wrote benchmark-speedup.svg")
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
