//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Benchmark struct {
	MSPerOp float64 `json:"ms_per_op"`
}

type Series struct {
	ID         string               `json:"id"`
	Label      string               `json:"label"`
	Benchmarks map[string]Benchmark `json:"benchmarks"`
}

type CrossLang struct {
	BunJSC    map[string]float64 `json:"bun_jsc"`
	Python313 map[string]float64 `json:"python_313"`
	Goja      map[string]float64 `json:"goja"`
}

type History struct {
	Metadata struct {
		Host          string   `json:"host"`
		NonComparable []string `json:"non_comparable_baseline"`
	} `json:"metadata"`
	Series        []Series  `json:"series"`
	CrossLanguage CrossLang `json:"cross_language"`
}

type Row struct {
	Name         string
	BaselineMS   float64
	CurrentMS    float64
	GojaMS       float64
	DeltaPct     float64
	GojaRatio    float64
	HasBaseline  bool
	CurrentWidth int
}

func main() {
	baseDir := "."
	if len(os.Args) > 1 {
		baseDir = os.Args[1]
	}

	data, err := os.ReadFile(filepath.Join(baseDir, "benchmark-history.json"))
	if err != nil {
		panic(err)
	}
	var h History
	if err := json.Unmarshal(data, &h); err != nil {
		panic(err)
	}

	baseline := findSeries(h, "baseline-stable")
	current := findSeries(h, "current")

	var rows []Row
	for name, cur := range current.Benchmarks {
		if cur.MSPerOp <= 0 {
			panic(fmt.Sprintf("current benchmark %s must be positive", name))
		}
		r := Row{Name: name, CurrentMS: cur.MSPerOp}
		comparable := true
		for _, excluded := range h.Metadata.NonComparable {
			if name == excluded {
				comparable = false
			}
		}
		if base, ok := baseline.Benchmarks[name]; ok && comparable {
			if base.MSPerOp <= 0 {
				panic(fmt.Sprintf("baseline benchmark %s must be positive", name))
			}
			r.BaselineMS = base.MSPerOp
			r.HasBaseline = true
			r.DeltaPct = ((cur.MSPerOp - base.MSPerOp) / base.MSPerOp) * 100
			r.CurrentWidth = int(math.Round((cur.MSPerOp / base.MSPerOp) * 600))
			if r.CurrentWidth > 600 {
				r.CurrentWidth = 600
			}
			if r.CurrentWidth < 10 {
				r.CurrentWidth = 10
			}
		}
		goja := requiredCross(h.CrossLanguage.Goja, "goja", name)
		r.GojaMS = goja
		r.GojaRatio = cur.MSPerOp / goja
		rows = append(rows, r)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].HasBaseline != rows[j].HasBaseline {
			return rows[i].HasBaseline
		}
		if rows[i].HasBaseline {
			return rows[i].DeltaPct < rows[j].DeltaPct
		}
		return rows[i].GojaRatio < rows[j].GojaRatio
	})

	rowHeight := 62
	headerHeight := 100
	summaryHeight := 80
	height := headerHeight + len(rows)*rowHeight + summaryHeight + 40

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="%d" viewBox="0 0 1200 %d">
<style>
:root { color-scheme: light dark; --bg:#f6f8fc;--panel:#fff;--row:#f0f4fb;--track:#dfe7f5;--border:#c7d3ea;--text:#172033;--muted:#55627c;--win:#23a566;--close:#f5a623;--gap:#e04848;--baseline:#7f93b8; }
@media(prefers-color-scheme:dark){:root{--bg:#0b1020;--panel:#11182b;--row:#0f1728;--track:#1c2740;--border:#25324f;--text:#e3ecff;--muted:#9db0d6;--win:#37c67e;--close:#f5a623;--gap:#f06060;--baseline:#7288b3;}}
svg{background:var(--bg)}.panel{fill:var(--panel);stroke:var(--border)}.row{fill:var(--row);stroke:var(--border)}.track{fill:var(--track)}.baseline{fill:var(--baseline)}.win{fill:var(--win)}.close{fill:var(--close)}.gap{fill:var(--gap)}
.text{fill:var(--text);font-family:Inter,system-ui,sans-serif}.muted{fill:var(--muted);font-family:Inter,system-ui,sans-serif}
.title{font-size:22px;font-weight:700}.subtitle{font-size:12px;font-weight:500}.label{font-size:12px;font-weight:600}.small{font-size:11px;font-weight:500}.tiny{font-size:10px}
</style>
`, height, height))

	b.WriteString(fmt.Sprintf(`<rect x="0" y="0" width="1200" height="%d" fill="var(--bg)"/>`, height))
	b.WriteString(fmt.Sprintf(`<rect class="panel" x="20" y="20" width="1160" height="%d" rx="14"/>`, height-40))
	b.WriteString(`<text class="text title" x="40" y="56">Joker benchmark improvements</text>`)
	b.WriteString(fmt.Sprintf(`<text class="muted subtitle" x="40" y="76">Generated from benchmark-history.json • Host: %s</text>`, h.Metadata.Host))

	y := headerHeight
	for _, r := range rows {
		b.WriteString(fmt.Sprintf(`<g transform="translate(32,%d)">`, y))
		b.WriteString(`<rect class="row" x="0" y="0" width="1136" height="54" rx="8"/>`)

		displayName := strings.ReplaceAll(r.Name, "_", " ")
		b.WriteString(fmt.Sprintf(`<text class="text label" x="10" y="20">%s</text>`, displayName))
		b.WriteString(fmt.Sprintf(`<text class="muted small" x="10" y="40">%.1f ms</text>`, r.CurrentMS))

		if r.HasBaseline {
			b.WriteString(`<rect class="track" x="180" y="10" width="600" height="14" rx="5"/>`)
			b.WriteString(`<rect class="baseline" x="180" y="10" width="600" height="14" rx="5"/>`)
			barClass := "win"
			if r.DeltaPct > -20 {
				barClass = "close"
			}
			if r.DeltaPct > 0 {
				barClass = "gap"
			}
			b.WriteString(fmt.Sprintf(`<rect class="%s" x="180" y="28" width="%d" height="14" rx="5"/>`, barClass, r.CurrentWidth))
			b.WriteString(fmt.Sprintf(`<text class="muted tiny" x="800" y="20">baseline: %.1f ms</text>`, r.BaselineMS))
			b.WriteString(fmt.Sprintf(`<text class="text label" x="800" y="40">%.1f%% %s</text>`, r.DeltaPct, speedupLabel(r.DeltaPct)))
		} else {
			barWidth := 600
			if r.GojaRatio > 0 && r.GojaRatio < 7 {
				barWidth = int(math.Round(r.GojaRatio / 7.0 * 600))
			}
			barClass := "win"
			if r.GojaRatio > 1 {
				barClass = "close"
			}
			if r.GojaRatio > 4 {
				barClass = "gap"
			}
			b.WriteString(`<rect class="track" x="180" y="18" width="600" height="14" rx="5"/>`)
			b.WriteString(fmt.Sprintf(`<rect class="%s" x="180" y="18" width="%d" height="14" rx="5"/>`, barClass, barWidth))
			b.WriteString(fmt.Sprintf(`<text class="muted tiny" x="800" y="22">Goja: %.2f ms</text>`, r.GojaMS))
			ratioLabel := fmt.Sprintf("%.1f× vs Goja", r.GojaRatio)
			if r.GojaRatio < 1 {
				ratioLabel = fmt.Sprintf("%.1f× faster ✅", 1/r.GojaRatio)
			}
			b.WriteString(fmt.Sprintf(`<text class="text label" x="800" y="40">%s</text>`, ratioLabel))
		}

		b.WriteString(`</g>`)
		y += rowHeight
	}

	b.WriteString(`</svg>`)

	if err := os.WriteFile(filepath.Join(baseDir, "benchmark-improvements.svg"), []byte(b.String()), 0644); err != nil {
		panic(err)
	}
	fmt.Println("wrote", filepath.Join(baseDir, "benchmark-improvements.svg"))

	generateCrossLanguageSVG(baseDir, h, current)
}

func generateCrossLanguageSVG(baseDir string, h History, current Series) {
	type CLRow struct {
		Name     string
		JokerMS  float64
		PythonMS float64
		GojaMS   float64
		VsPython float64
		VsGoja   float64
	}

	var rows []CLRow
	for name, bench := range current.Benchmarks {
		if bench.MSPerOp <= 0 {
			panic(fmt.Sprintf("current benchmark %s must be positive", name))
		}
		py := requiredCross(h.CrossLanguage.Python313, "python_313", name)
		gj := requiredCross(h.CrossLanguage.Goja, "goja", name)
		rows = append(rows, CLRow{Name: name, JokerMS: bench.MSPerOp, PythonMS: py, GojaMS: gj, VsPython: bench.MSPerOp / py, VsGoja: bench.MSPerOp / gj})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].VsPython < rows[j].VsPython
	})

	rowHeight := 54
	headerHeight := 100
	height := headerHeight + len(rows)*rowHeight + 60

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="%d" viewBox="0 0 1200 %d">
<style>
:root { color-scheme: light dark; --bg:#f6f8fc;--panel:#fff;--row:#f0f4fb;--track:#dfe7f5;--border:#c7d3ea;--text:#172033;--muted:#55627c;--win:#23a566;--close:#f5a623;--gap:#e04848;--baseline:#7f93b8;--py:#306998;--goja:#c96b2e; }
@media(prefers-color-scheme:dark){:root{--bg:#0b1020;--panel:#11182b;--row:#0f1728;--track:#1c2740;--border:#25324f;--text:#e3ecff;--muted:#9db0d6;--win:#37c67e;--close:#f5a623;--gap:#f06060;--baseline:#7288b3;--py:#5b9bd5;--goja:#e8a04f;}}
svg{background:var(--bg)}.panel{fill:var(--panel);stroke:var(--border)}.row{fill:var(--row);stroke:var(--border)}
.text{fill:var(--text);font-family:Inter,system-ui,sans-serif}.muted{fill:var(--muted);font-family:Inter,system-ui,sans-serif}
.title{font-size:22px;font-weight:700}.subtitle{font-size:12px;font-weight:500}.label{font-size:12px;font-weight:600}.small{font-size:11px;font-weight:500}.tiny{font-size:10px}
.win{fill:var(--win)}.close{fill:var(--close)}.gap{fill:var(--gap)}.py{fill:var(--py)}.goja{fill:var(--goja)}
</style>
`, height, height))

	b.WriteString(fmt.Sprintf(`<rect x="0" y="0" width="1200" height="%d" fill="var(--bg)"/>`, height))
	b.WriteString(fmt.Sprintf(`<rect class="panel" x="20" y="20" width="1160" height="%d" rx="14"/>`, height-40))
	b.WriteString(`<text class="text title" x="40" y="56">Joker vs Python 3.14 vs Goja — CLBG benchmarks</text>`)

	winsP, winsG := 0, 0
	for _, r := range rows {
		if r.VsPython < 1 {
			winsP++
		}
		if r.VsGoja < 1 {
			winsG++
		}
	}
	b.WriteString(fmt.Sprintf(`<text class="muted subtitle" x="40" y="76">Beat Python: %d/%d • Beat Goja: %d/%d</text>`, winsP, len(rows), winsG, len(rows)))

	b.WriteString(`<text class="muted tiny" x="200" y="94">Joker ms</text>`)
	b.WriteString(`<text class="muted tiny" x="280" y="94">Python ms</text>`)
	b.WriteString(`<text class="muted tiny" x="370" y="94">Goja ms</text>`)
	b.WriteString(`<text class="muted tiny" x="450" y="94">vs Python</text>`)
	b.WriteString(`<text class="muted tiny" x="560" y="94">vs Goja</text>`)
	b.WriteString(`<text class="muted tiny" x="660" y="94">ratio bar (log scale)</text>`)

	y := headerHeight
	for _, r := range rows {
		b.WriteString(fmt.Sprintf(`<g transform="translate(32,%d)">`, y))
		b.WriteString(`<rect class="row" x="0" y="0" width="1136" height="46" rx="6"/>`)

		displayName := strings.ReplaceAll(r.Name, "_", "-")
		b.WriteString(fmt.Sprintf(`<text class="text label" x="10" y="28">%s</text>`, displayName))
		b.WriteString(fmt.Sprintf(`<text class="muted small" x="170" y="28">%.3f</text>`, r.JokerMS))
		b.WriteString(fmt.Sprintf(`<text class="muted small" x="260" y="28">%.2f</text>`, r.PythonMS))
		b.WriteString(fmt.Sprintf(`<text class="muted small" x="350" y="28">%.2f</text>`, r.GojaMS))

		pyClass := "win"
		if r.VsPython >= 1 && r.VsPython < 2 {
			pyClass = "close"
		}
		if r.VsPython >= 2 {
			pyClass = "gap"
		}
		b.WriteString(fmt.Sprintf(`<text class="%s small" x="440" y="28">%.2f×</text>`, pyClass, r.VsPython))

		gjClass := "win"
		if r.VsGoja >= 1 && r.VsGoja < 2 {
			gjClass = "close"
		}
		if r.VsGoja >= 2 {
			gjClass = "gap"
		}
		b.WriteString(fmt.Sprintf(`<text class="%s small" x="550" y="28">%.2f×</text>`, gjClass, r.VsGoja))

		barMax := 500.0
		logRatio := math.Log2(math.Max(r.VsPython, 0.01))
		barWidth := int(math.Round((logRatio + 6) / 11.0 * barMax))
		if barWidth < 2 {
			barWidth = 2
		}
		if barWidth > int(barMax) {
			barWidth = int(barMax)
		}
		b.WriteString(fmt.Sprintf(`<rect fill="var(--track)" x="640" y="14" width="%d" height="18" rx="4"/>`, int(barMax)))
		b.WriteString(fmt.Sprintf(`<rect class="%s" x="640" y="14" width="%d" height="18" rx="4" opacity="0.8"/>`, pyClass, barWidth))
		oneX := int(math.Round(6.0 / 11.0 * barMax))
		b.WriteString(fmt.Sprintf(`<line x1="%d" y1="12" x2="%d" y2="34" stroke="var(--text)" stroke-width="1" opacity="0.3"/>`, 640+oneX, 640+oneX))

		b.WriteString(`</g>`)
		y += rowHeight
	}

	b.WriteString(`</svg>`)
	if err := os.WriteFile(filepath.Join(baseDir, "benchmark-cross-language.svg"), []byte(b.String()), 0644); err != nil {
		panic(err)
	}
	fmt.Println("wrote", filepath.Join(baseDir, "benchmark-cross-language.svg"))
}

func findSeries(h History, id string) Series {
	for _, s := range h.Series {
		if s.ID == id {
			return s
		}
	}
	panic("missing benchmark series: " + id)
}

func requiredCross(values map[string]float64, runtime string, name string) float64 {
	if values == nil {
		panic("missing cross-language runtime: " + runtime)
	}
	v, ok := values[name]
	if !ok || v <= 0 {
		panic(fmt.Sprintf("missing positive %s benchmark value for %s", runtime, name))
	}
	return v
}

func speedupLabel(pct float64) string {
	if pct <= -50 {
		return "🚀"
	}
	if pct <= -20 {
		return "⬇️"
	}
	if pct >= 0 {
		return "⬆️"
	}
	return ""
}
