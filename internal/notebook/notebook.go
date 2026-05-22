package notebook

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	core "github.com/rcarmo/go-joker/core"
	corereader "github.com/rcarmo/go-joker/core/reader"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

type Notebook struct {
	Format    string
	Version   int
	Title     string
	CreatedAt string
	UpdatedAt string
	Cells     []Cell
}

type Cell struct {
	ID             string
	Kind           string
	Name           string
	DependsOn      []string
	Source         string
	ExecutionCount int
	State          string
	Outputs        []Output
}

type Output struct {
	Type     string
	Text     string
	MIME     string
	Data     string
	Encoding string
	Renderer string
	Spec     string
	Source   string
}

func New(title string) Notebook {
	now := time.Now().UTC().Format(time.RFC3339)
	return Notebook{Format: "joker/notebook", Version: 1, Title: title, CreatedAt: now, UpdatedAt: now}
}

func Load(path string) (Notebook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Notebook{}, err
	}
	reader := core.NewReader(bufio.NewReader(bytes.NewReader(data)), path)
	obj, err := core.TryRead(reader)
	if err != nil {
		return Notebook{}, err
	}
	m, ok := obj.(coretypes.Map)
	if !ok {
		return Notebook{}, fmt.Errorf("notebook root must be an EDN map")
	}
	nb := New(lookupString(m, "title"))
	nb.Format = strings.TrimPrefix(lookupKeywordOrString(m, "format"), ":")
	if nb.Format != "joker/notebook" {
		return Notebook{}, fmt.Errorf("notebook :format must be :joker/notebook")
	}
	nb.Version = lookupInt(m, "version")
	if nb.Version == 0 {
		nb.Version = 1
	}
	nb.CreatedAt = lookupString(m, "created-at")
	nb.UpdatedAt = lookupString(m, "updated-at")
	nb.Cells = parseCells(lookup(m, "cells"))
	return nb, nil
}

func Save(path string, nb Notebook) error { return os.WriteFile(path, []byte(Encode(nb)), 0644) }

func Run(nb *Notebook) {
	for i := range nb.Cells {
		if nb.Cells[i].Kind == "code" || nb.Cells[i].Kind == "" {
			EvaluateCell(&nb.Cells[i])
		}
	}
	nb.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}

func EvaluateCell(c *Cell) {
	c.ExecutionCount++
	var out, errb bytes.Buffer
	oldOut, oldErr := core.Stdout, core.Stderr
	core.Stdout, core.Stderr = &out, &errb
	err := core.ProcessReader(core.NewReader(bufio.NewReader(strings.NewReader(c.Source)), "<notebook-cell>"), "", corereader.PrintIfNotNilPhase)
	core.Stdout, core.Stderr = oldOut, oldErr
	c.Outputs = nil
	if out.Len() > 0 {
		c.Outputs = append(c.Outputs, Output{Type: "stdout", Text: out.String()})
	}
	if errb.Len() > 0 {
		c.Outputs = append(c.Outputs, Output{Type: "stderr", Text: errb.String()})
	}
	if err != nil {
		c.State = "error"
		c.Outputs = append(c.Outputs, Output{Type: "error", Text: err.Error()})
	} else {
		c.State = "ok"
	}
}

func ExportMarkdown(w io.Writer, nb Notebook) error {
	if nb.Title != "" {
		fmt.Fprintf(w, "# %s\n\n", nb.Title)
	}
	for _, c := range nb.Cells {
		switch c.Kind {
		case "markdown":
			fmt.Fprintln(w, c.Source, "")
		default:
			fmt.Fprintln(w, "```clojure")
			fmt.Fprintln(w, c.Source)
			fmt.Fprintln(w, "```")
			fmt.Fprintln(w)
		}
		for _, o := range c.Outputs {
			switch o.Type {
			case "stdout", "stderr", "error":
				fmt.Fprintf(w, "```text\n%s\n```\n\n", strings.TrimRight(o.Text, "\n"))
			case "svg":
				fmt.Fprintln(w, o.Source, "")
			case "image":
				fmt.Fprintf(w, "![image](data:%s;base64,%s)\n\n", o.MIME, o.Data)
			default:
				if o.Text != "" {
					fmt.Fprintln(w, o.Text, "")
				}
			}
		}
	}
	return nil
}

func Encode(nb Notebook) string {
	var b strings.Builder
	fmt.Fprintf(&b, "{:format :joker/notebook\n :version %d\n", nb.Version)
	fmt.Fprintf(&b, " :title %s\n :created-at %s\n :updated-at %s\n :cells [\n", q(nb.Title), q(nb.CreatedAt), q(nb.UpdatedAt))
	for _, c := range nb.Cells {
		fmt.Fprintf(&b, "  {:id %s :kind :%s", q(c.ID), emptyDefault(c.Kind, "code"))
		if c.Name != "" {
			fmt.Fprintf(&b, " :name %s", q(c.Name))
		}
		fmt.Fprintf(&b, " :depends-on [%s] :source %s :execution-count %d :state :%s :outputs [", quoteList(c.DependsOn), q(c.Source), c.ExecutionCount, emptyDefault(c.State, "idle"))
		for _, o := range c.Outputs {
			fmt.Fprintf(&b, "{:type :%s", emptyDefault(o.Type, "value"))
			if o.Text != "" {
				fmt.Fprintf(&b, " :text %s", q(o.Text))
			}
			if o.MIME != "" {
				fmt.Fprintf(&b, " :mime %s", q(o.MIME))
			}
			if o.Encoding != "" {
				fmt.Fprintf(&b, " :encoding :%s", o.Encoding)
			}
			if o.Renderer != "" {
				fmt.Fprintf(&b, " :renderer :%s", o.Renderer)
			}
			if o.Data != "" {
				fmt.Fprintf(&b, " :data %s", q(o.Data))
			}
			if o.Spec != "" {
				fmt.Fprintf(&b, " :spec %s", q(o.Spec))
			}
			if o.Source != "" {
				fmt.Fprintf(&b, " :source %s", q(o.Source))
			}
			b.WriteString("}")
		}
		b.WriteString("]}\n")
	}
	b.WriteString("]}\n")
	return b.String()
}

func Serve(addr, path string, open bool) error {
	nb, err := Load(path)
	if err != nil {
		nb = New(path)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { _ = page.Execute(w, nb) })
	mux.HandleFunc("/api/notebook", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	mux.HandleFunc("/api/evaluate-all", func(w http.ResponseWriter, r *http.Request) {
		Run(&nb)
		_ = Save(path, nb)
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	fmt.Printf("Serving Joker notebook at http://%s/\n", addr)
	if open {
		fmt.Printf("Open http://%s/ in your browser.\n", addr)
	}
	return http.ListenAndServe(addr, mux)
}

var page = template.Must(template.New("nb").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Joker Notebook</title><style>body{font-family:system-ui;margin:2rem auto;max-width:1100px;line-height:1.4}textarea{width:100%;min-height:8rem;font-family:ui-monospace,monospace}.cell{border:1px solid #ccc;border-radius:8px;padding:1rem;margin:1rem 0}pre{background:#f3f4f6;padding:1rem;overflow:auto}.kw{color:#7c3aed;font-weight:600}@media(prefers-color-scheme:dark){body{background:#0d1117;color:#e6edf3}.cell{border-color:#30363d}pre{background:#161b22}}</style></head><body><h1>{{.Title}}</h1><p><button onclick="fetch('/api/evaluate-all',{method:'POST'}).then(r=>r.text()).then(t=>document.getElementById('raw').textContent=t)">Evaluate all</button></p>{{range .Cells}}<div class="cell"><b>{{.Kind}}</b>{{if .Name}} — {{.Name}}{{end}}<textarea>{{.Source}}</textarea>{{range .Outputs}}<pre>{{.Text}}</pre>{{end}}</div>{{end}}<h2>Raw notebook</h2><pre id="raw"></pre></body></html>`))

func lookup(m coretypes.Map, key string) coretypes.Object {
	ok, v := m.Get(coretypes.MakeKeyword(core.STRINGS.Intern, key))
	if ok {
		return v
	}
	return nil
}
func lookupString(m coretypes.Map, key string) string {
	if s, ok := lookup(m, key).(coretypes.String); ok {
		return s.S
	}
	return ""
}
func lookupKeywordOrString(m coretypes.Map, key string) string {
	v := lookup(m, key)
	if v == nil {
		return ""
	}
	return v.ToString(false)
}
func lookupInt(m coretypes.Map, key string) int {
	if n, ok := lookup(m, key).(coretypes.Int); ok {
		return n.I
	}
	return 0
}
func parseCells(obj coretypes.Object) []Cell {
	seqable, ok := obj.(coretypes.Seqable)
	if !ok {
		return nil
	}
	var out []Cell
	for s := seqable.Seq(); s != nil && !s.IsEmpty(); s = s.Rest() {
		if m, ok := s.First().(coretypes.Map); ok {
			out = append(out, parseCell(m))
		}
	}
	return out
}
func parseCell(m coretypes.Map) Cell {
	c := Cell{ID: lookupString(m, "id"), Kind: strings.TrimPrefix(lookupKeywordOrString(m, "kind"), ":"), Name: lookupString(m, "name"), Source: lookupString(m, "source"), ExecutionCount: lookupInt(m, "execution-count"), State: strings.TrimPrefix(lookupKeywordOrString(m, "state"), ":"), DependsOn: parseStrings(lookup(m, "depends-on")), Outputs: parseOutputs(lookup(m, "outputs"))}
	return c
}
func parseStrings(obj coretypes.Object) []string {
	seqable, ok := obj.(coretypes.Seqable)
	if !ok {
		return nil
	}
	var out []string
	for s := seqable.Seq(); s != nil && !s.IsEmpty(); s = s.Rest() {
		if x, ok := s.First().(coretypes.String); ok {
			out = append(out, x.S)
		}
	}
	return out
}
func parseOutputs(obj coretypes.Object) []Output {
	seqable, ok := obj.(coretypes.Seqable)
	if !ok {
		return nil
	}
	var out []Output
	for s := seqable.Seq(); s != nil && !s.IsEmpty(); s = s.Rest() {
		if m, ok := s.First().(coretypes.Map); ok {
			out = append(out, Output{Type: strings.TrimPrefix(lookupKeywordOrString(m, "type"), ":"), Text: lookupString(m, "text"), MIME: lookupString(m, "mime"), Data: lookupString(m, "data"), Encoding: strings.TrimPrefix(lookupKeywordOrString(m, "encoding"), ":"), Renderer: strings.TrimPrefix(lookupKeywordOrString(m, "renderer"), ":"), Spec: lookupString(m, "spec"), Source: lookupString(m, "source")})
		}
	}
	return out
}
func q(s string) string { return strconv.Quote(s) }
func quoteList(xs []string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = q(x)
	}
	return strings.Join(parts, " ")
}
func emptyDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
func EncodeImage(mime string, data []byte) Output {
	return Output{Type: "image", MIME: mime, Encoding: "base64", Data: base64.StdEncoding.EncodeToString(data)}
}
func Downstream(nb Notebook, name string) []Cell {
	seen := map[string]bool{name: true}
	var out []Cell
	changed := true
	for changed {
		changed = false
		for _, c := range nb.Cells {
			if seen[c.Name] {
				continue
			}
			for _, d := range c.DependsOn {
				if seen[d] {
					seen[c.Name] = true
					out = append(out, c)
					changed = true
					break
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
