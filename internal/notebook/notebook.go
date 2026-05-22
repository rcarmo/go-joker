package notebook

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
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
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
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

type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type DependencyGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type Status struct {
	Title       string `json:"title"`
	CellCount   int    `json:"cellCount"`
	OutputCount int    `json:"outputCount"`
	Bytes       int    `json:"bytes"`
	Warning     string `json:"warning,omitempty"`
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
	return Decode(data, path)
}

func Save(path string, nb Notebook) error { return os.WriteFile(path, []byte(Encode(nb)), 0644) }

func BuildStatus(nb Notebook) Status {
	status := Status{Title: nb.Title, CellCount: len(nb.Cells), Bytes: len(Encode(nb))}
	for _, c := range nb.Cells {
		status.OutputCount += len(c.Outputs)
	}
	if status.Bytes > 10*1024*1024 {
		status.Warning = "notebook EDN exceeds 10 MB; consider pruning inline outputs"
	}
	return status
}

func Run(nb *Notebook) {
	for i := range nb.Cells {
		if nb.Cells[i].Kind == "code" || nb.Cells[i].Kind == "" {
			EvaluateCell(&nb.Cells[i])
		}
	}
	nb.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}

func EvaluateDownstream(nb *Notebook, name string) []string {
	if len(DependencyCycles(*nb)) > 0 {
		return nil
	}
	cells := Downstream(*nb, name)
	ids := make([]string, 0, len(cells))
	wanted := map[string]bool{}
	for _, c := range cells {
		wanted[c.ID] = true
	}
	for i := range nb.Cells {
		if wanted[nb.Cells[i].ID] && (nb.Cells[i].Kind == "code" || nb.Cells[i].Kind == "") {
			EvaluateCell(&nb.Cells[i])
			ids = append(ids, nb.Cells[i].ID)
		}
	}
	nb.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return ids
}

func EvaluateCell(c *Cell) {
	c.ExecutionCount++
	var out, errb bytes.Buffer
	oldOut, oldErr := core.Stdout, core.Stderr
	core.Stdout, core.Stderr = &out, &errb
	result, err := evalSource(c.Source)
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
		return
	}
	if result != nil {
		c.Outputs = append(c.Outputs, outputFromObject(result))
	}
	c.State = "ok"
}

func evalSource(source string) (coretypes.Object, error) {
	installNotebookHelpers()
	reader := core.NewReader(bufio.NewReader(strings.NewReader(source)), "<notebook-cell>")
	ctx := &core.ParseContext{GlobalEnv: core.GLOBAL_ENV}
	var result coretypes.Object
	for {
		obj, err := core.TryRead(reader)
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return nil, err
		}
		expr, err := core.TryParse(obj, ctx)
		if err != nil {
			return nil, err
		}
		result, err = core.TryEval(expr)
		if err != nil {
			return nil, err
		}
	}
}

func installNotebookHelpers() {
	ns := core.GLOBAL_ENV.EnsureSymbolIsLib(coretypes.MakeSymbol(core.STRINGS.Intern, "joker.notebook"))
	intern := func(name string, fn core.ProcFn) {
		if ns.Resolve(name) != nil {
			return
		}
		ns.InternVar(name, core.Proc{Name: "notebook/" + name, Fn: fn}, core.MakeMeta(nil, "Notebook rich output helper.", "1.0"))
	}
	intern("chart", func(args []coretypes.Object) coretypes.Object {
		return richOutputMap("chart", "echarts", "spec", firstArgString(args))
	})
	intern("svg", func(args []coretypes.Object) coretypes.Object {
		return richOutputMap("svg", "", "source", firstArgString(args))
	})
	intern("mermaid", func(args []coretypes.Object) coretypes.Object {
		return richOutputMap("diagram", "mermaid", "source", firstArgString(args))
	})
	intern("dot", func(args []coretypes.Object) coretypes.Object {
		return richOutputMap("diagram", "dot", "source", firstArgString(args))
	})
	intern("graph", func(args []coretypes.Object) coretypes.Object {
		return richOutputMap("graph", "graph-json", "source", firstArgString(args))
	})
	intern("image", func(args []coretypes.Object) coretypes.Object {
		mime := "image/png"
		data := ""
		if len(args) > 0 {
			mime = firstArgString(args[:1])
		}
		if len(args) > 1 {
			data = firstArgString(args[1:2])
		}
		m := richOutputMap("image", "", "data", data)
		m.Add(coretypes.MakeKeyword(core.STRINGS.Intern, "mime"), coretypes.MakeString(mime))
		m.Add(coretypes.MakeKeyword(core.STRINGS.Intern, "encoding"), coretypes.MakeKeyword(core.STRINGS.Intern, "base64"))
		return m
	})
}

func firstArgString(args []coretypes.Object) string {
	if len(args) == 0 || args[0] == nil {
		return ""
	}
	if s, ok := args[0].(coretypes.String); ok {
		return s.S
	}
	return args[0].ToString(false)
}

func richOutputMap(outputType, renderer, valueKey, value string) *corecollections.ArrayMap {
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(core.STRINGS.Intern, "notebook/output"), coretypes.MakeKeyword(core.STRINGS.Intern, outputType))
	m.Add(coretypes.MakeKeyword(core.STRINGS.Intern, "type"), coretypes.MakeKeyword(core.STRINGS.Intern, outputType))
	if renderer != "" {
		m.Add(coretypes.MakeKeyword(core.STRINGS.Intern, "renderer"), coretypes.MakeKeyword(core.STRINGS.Intern, renderer))
	}
	m.Add(coretypes.MakeKeyword(core.STRINGS.Intern, valueKey), coretypes.MakeString(value))
	return m
}

func outputFromObject(obj coretypes.Object) Output {
	if m, ok := obj.(coretypes.Map); ok {
		if out, ok := richOutputFromMap(m); ok {
			return out
		}
	}
	return Output{Type: "value", MIME: "text/edn", Text: obj.ToString(true)}
}

func richOutputFromMap(m coretypes.Map) (Output, bool) {
	typeName := strings.TrimPrefix(mapString(m, "type"), ":")
	if typeName == "" {
		typeName = strings.TrimPrefix(mapString(m, "notebook/output"), ":")
	}
	if typeName == "" {
		return Output{}, false
	}
	out := Output{Type: typeName, Text: mapString(m, "text"), MIME: mapString(m, "mime"), Data: mapString(m, "data"), Encoding: strings.TrimPrefix(mapString(m, "encoding"), ":"), Renderer: strings.TrimPrefix(mapString(m, "renderer"), ":"), Spec: mapString(m, "spec"), Source: mapString(m, "source")}
	if out.Source == "" {
		out.Source = mapString(m, "svg")
	}
	return out, true
}

func mapString(m coretypes.Map, key string) string {
	if v := lookup(m, key); v != nil {
		if s, ok := v.(coretypes.String); ok {
			return s.S
		}
		return v.ToString(false)
	}
	return ""
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
			writeMarkdownOutput(w, o)
		}
	}
	return nil
}

func writeMarkdownOutput(w io.Writer, o Output) {
	switch o.Type {
	case "stdout", "stderr", "error":
		fmt.Fprintf(w, "```text\n%s\n```\n\n", strings.TrimRight(o.Text, "\n"))
	case "value":
		fmt.Fprintf(w, "```edn\n%s\n```\n\n", strings.TrimRight(o.Text, "\n"))
	case "svg":
		fmt.Fprintln(w, o.Source, "")
	case "image":
		fmt.Fprintf(w, "![image](data:%s;base64,%s)\n\n", o.MIME, o.Data)
	case "chart":
		fmt.Fprintf(w, "```json\n%s\n```\n\n", strings.TrimSpace(o.Spec))
	case "diagram":
		lang := o.Renderer
		if lang == "" {
			lang = "text"
		}
		fmt.Fprintf(w, "```%s\n%s\n```\n\n", lang, strings.TrimSpace(o.Source))
	case "graph":
		fmt.Fprintf(w, "```json\n%s\n```\n\n", strings.TrimSpace(o.Source))
	default:
		if o.Text != "" {
			fmt.Fprintln(w, o.Text, "")
		}
	}
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
	fmt.Printf("Serving Joker notebook at http://%s/\n", addr)
	if open {
		fmt.Printf("Open http://%s/ in your browser.\n", addr)
	}
	return http.ListenAndServe(addr, Handler(path))
}

func Handler(path string) http.Handler {
	nb, err := Load(path)
	if err != nil {
		nb = New(path)
		nb.Cells = []Cell{{ID: "cell-1", Kind: "code", Source: "(+ 1 2)", State: "idle"}}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { _ = page.Execute(w, nb) })
	mux.HandleFunc("/api/notebook", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BuildStatus(nb))
	})
	mux.HandleFunc("/api/export/markdown", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_ = ExportMarkdown(w, nb)
	})
	mux.HandleFunc("/api/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		loaded, err := Decode(body, "<request>")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		nb = loaded
		if err := saveCurrent(path, &nb); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	mux.HandleFunc("/api/save-sources", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := applySourceUpdate(r.Body, &nb); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveCurrent(path, &nb); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	mux.HandleFunc("/api/clear-outputs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			for i := range nb.Cells {
				nb.Cells[i].Outputs = nil
				nb.Cells[i].State = "idle"
			}
		} else if cell, ok := findCell(&nb, id); ok {
			cell.Outputs = nil
			cell.State = "idle"
		} else {
			http.Error(w, "cell not found", http.StatusNotFound)
			return
		}
		if err := saveCurrent(path, &nb); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	mux.HandleFunc("/api/cell", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			kind := r.URL.Query().Get("kind")
			if kind == "" {
				kind = "code"
			}
			cell := Cell{ID: nextCellID(nb), Kind: kind, State: "idle"}
			if kind == "markdown" {
				cell.Source = "Markdown"
			} else {
				cell.Source = "(+ 1 2)"
			}
			nb.Cells = append(nb.Cells, cell)
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "missing id", http.StatusBadRequest)
				return
			}
			if !deleteCell(&nb, id) {
				http.Error(w, "cell not found", http.StatusNotFound)
				return
			}
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := saveCurrent(path, &nb); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	mux.HandleFunc("/api/reorder", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := applyReorder(r.Body, &nb); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveCurrent(path, &nb); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	mux.HandleFunc("/api/evaluate-cell", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		cell, ok := findCell(&nb, id)
		if !ok {
			http.Error(w, "cell not found", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 {
			cell.Source = string(body)
		}
		EvaluateCell(cell)
		_ = saveCurrent(path, &nb)
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	mux.HandleFunc("/api/dependencies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"cycles": DependencyCycles(nb), "graph": BuildDependencyGraph(nb)})
	})
	mux.HandleFunc("/api/evaluate-downstream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}
		_ = applySourceUpdate(r.Body, &nb)
		if cycles := DependencyCycles(nb); len(cycles) > 0 {
			http.Error(w, fmt.Sprintf("dependency cycle: %v", cycles), http.StatusBadRequest)
			return
		}
		_ = EvaluateDownstream(&nb, name)
		_ = saveCurrent(path, &nb)
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	mux.HandleFunc("/api/evaluate-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = applySourceUpdate(r.Body, &nb)
		Run(&nb)
		_ = saveCurrent(path, &nb)
		w.Header().Set("Content-Type", "application/edn")
		fmt.Fprint(w, Encode(nb))
	})
	return mux
}

func Decode(data []byte, filename string) (Notebook, error) {
	reader := core.NewReader(bufio.NewReader(bytes.NewReader(data)), filename)
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

func findCell(nb *Notebook, id string) (*Cell, bool) {
	for i := range nb.Cells {
		if nb.Cells[i].ID == id {
			return &nb.Cells[i], true
		}
	}
	return nil, false
}

func saveCurrent(path string, nb *Notebook) error {
	nb.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return Save(path, *nb)
}

type sourceUpdate struct {
	Title string `json:"title"`
	Cells []struct {
		ID        string   `json:"id"`
		Kind      string   `json:"kind"`
		Name      string   `json:"name"`
		DependsOn []string `json:"dependsOn"`
		Source    string   `json:"source"`
	} `json:"cells"`
}

func nextCellID(nb Notebook) string {
	max := 0
	for _, c := range nb.Cells {
		if strings.HasPrefix(c.ID, "cell-") {
			if n, err := strconv.Atoi(strings.TrimPrefix(c.ID, "cell-")); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("cell-%d", max+1)
}

func deleteCell(nb *Notebook, id string) bool {
	for i := range nb.Cells {
		if nb.Cells[i].ID == id {
			nb.Cells = append(nb.Cells[:i], nb.Cells[i+1:]...)
			return true
		}
	}
	return false
}

type reorderUpdate struct {
	IDs []string `json:"ids"`
}

func applyReorder(r io.Reader, nb *Notebook) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	var update reorderUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		return err
	}
	if len(update.IDs) != len(nb.Cells) {
		return fmt.Errorf("reorder requires exactly %d ids", len(nb.Cells))
	}
	byID := map[string]Cell{}
	for _, c := range nb.Cells {
		byID[c.ID] = c
	}
	reordered := make([]Cell, 0, len(nb.Cells))
	seen := map[string]bool{}
	for _, id := range update.IDs {
		c, ok := byID[id]
		if !ok || seen[id] {
			return fmt.Errorf("invalid reorder id %q", id)
		}
		seen[id] = true
		reordered = append(reordered, c)
	}
	nb.Cells = reordered
	return nil
}

func applySourceUpdate(r io.Reader, nb *Notebook) error {
	if r == nil {
		return nil
	}
	body, err := io.ReadAll(r)
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return err
	}
	var update sourceUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		return err
	}
	if update.Title != "" {
		nb.Title = update.Title
	}
	for _, c := range update.Cells {
		if cell, ok := findCell(nb, c.ID); ok {
			if c.Kind != "" {
				cell.Kind = c.Kind
			}
			cell.Name = c.Name
			cell.DependsOn = c.DependsOn
			cell.Source = c.Source
		}
	}
	return nil
}

var page = template.Must(template.New("nb").Funcs(template.FuncMap{"join": strings.Join}).Parse(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>Joker Notebook</title>
<style>
:root{color-scheme:light dark;--bg:#fff;--fg:#172033;--muted:#667085;--panel:#f8fafc;--border:#d0d7de;--code:#f3f4f6;--kw:#7c3aed;--sym:#0369a1;--str:#15803d;--err:#b91c1c;--accent:#2563eb}
@media(prefers-color-scheme:dark){:root{--bg:#0d1117;--fg:#e6edf3;--muted:#8b949e;--panel:#111827;--border:#30363d;--code:#161b22;--kw:#c084fc;--sym:#7dd3fc;--str:#86efac;--err:#fca5a5;--accent:#60a5fa}}
html[data-theme="light"]{color-scheme:light;--bg:#fff;--fg:#172033;--muted:#667085;--panel:#f8fafc;--border:#d0d7de;--code:#f3f4f6;--kw:#7c3aed;--sym:#0369a1;--str:#15803d;--err:#b91c1c;--accent:#2563eb}
html[data-theme="dark"]{color-scheme:dark;--bg:#0d1117;--fg:#e6edf3;--muted:#8b949e;--panel:#111827;--border:#30363d;--code:#161b22;--kw:#c084fc;--sym:#7dd3fc;--str:#86efac;--err:#fca5a5;--accent:#60a5fa}
body{background:var(--bg);color:var(--fg);font-family:system-ui,sans-serif;margin:2rem auto;max-width:1180px;line-height:1.45;padding:0 1rem}.toolbar{display:flex;gap:.35rem;flex-wrap:wrap;align-items:center}.spacer{flex:1}button{padding:.4rem .7rem;margin:.15rem;border:1px solid var(--border);border-radius:6px;background:var(--panel);color:var(--fg)}input,select{background:var(--code);color:var(--fg);border:1px solid var(--border);border-radius:5px;padding:.25rem}textarea{width:100%;min-height:8rem;font-family:ui-monospace,monospace;background:var(--code);color:var(--fg);border:1px solid var(--border);border-radius:6px;padding:.7rem}.cell{position:relative;border:1px solid var(--border);border-left:8px solid var(--accent);border-radius:10px;padding:1rem 1rem 1rem 1.25rem;margin:1rem 0;background:var(--panel);box-shadow:0 1px 2px #0001}.cell:before{content:attr(data-count);position:absolute;left:-3.4rem;top:1rem;color:var(--muted);font-family:ui-monospace,monospace;font-size:.8rem}.cell-state-ok{border-left-color:#16a34a}.cell-state-error{border-left-color:var(--err)}.cell-state-idle{border-left-color:var(--border)}.cell-header{display:flex;gap:.4rem;align-items:center;flex-wrap:wrap}.state-pill{border:1px solid var(--border);border-radius:999px;padding:.1rem .45rem;font-size:.75rem;color:var(--muted)}.meta-row{display:flex;flex-wrap:wrap;gap:.7rem;margin:.6rem 0}#notebook-log{border:1px solid var(--border);border-radius:6px;padding:.5rem;margin:.5rem 0;color:var(--muted);background:var(--panel)}#notebook-log.err{color:var(--err)}details.outputs{margin-top:.8rem}details.outputs summary{cursor:pointer;color:var(--muted)}pre{background:var(--code);padding:1rem;overflow:auto;border-radius:6px}.meta{color:var(--muted);font-size:.9rem}.kw{color:var(--kw);font-weight:700}.sym{color:var(--sym)}.str{color:var(--str)}.err{color:var(--err)}.output{border-left:4px solid var(--border);padding-left:.8rem;margin-top:.8rem}.chart,.diagram,.graph{min-height:180px;border:1px solid var(--border);border-radius:6px;padding:1rem;white-space:pre-wrap;background:var(--bg)}.chart svg,.graph svg{max-width:100%;height:auto}.bar{fill:var(--accent)}.axis{stroke:var(--border)}.node{fill:var(--panel);stroke:var(--accent)}.edge{stroke:var(--muted);marker-end:url(#arrow)}
</style>
</head>
<body>
<h1><input id="notebook-title" value="{{.Title}}" style="font-size:1.5rem;width:70%"></h1>
<p class="meta">Trusted local Joker execution. File is read/written by this server only.</p><p id="notebook-status" class="meta"></p><div id="notebook-dirty" class="meta">Saved</div><div id="notebook-log">Ready.</div>
<p class="toolbar"><button onclick="evaluateAll()">Evaluate all</button><button onclick="saveNotebook()">Save</button><button onclick="exportMarkdown()">Export Markdown</button><button onclick="loadRawEdn()">Load raw EDN</button><button onclick="clearOutputs('')">Clear all outputs</button><button onclick="checkDeps()">Check deps</button><button onclick="showDependencyGraph()">Show dependency graph</button><button onclick="addCell('code')">Add code</button><button onclick="addCell('markdown')">Add Markdown</button><span class="spacer"></span><button onclick="setTheme('light')">Light</button><button onclick="setTheme('dark')">Dark</button><button onclick="setTheme('auto')">Auto</button></p>
<div id="dependency-graph" class="graph" style="display:none"></div>
<div id="cells">{{range .Cells}}<div class="cell cell-state-{{.State}}" data-id="{{.ID}}" data-name="{{.Name}}" data-count="In[{{.ExecutionCount}}]"><div class="cell-header"><b>{{.Kind}}</b><span class="state-pill">{{if .State}}{{.State}}{{else}}idle{{end}}</span><span class="meta">{{.ID}}{{if .Name}} · {{.Name}}{{end}}{{if .DependsOn}} · depends on {{.DependsOn}}{{end}}</span><button onclick="evaluateCell('{{.ID}}')">Evaluate</button>{{if .Name}}<button onclick="evaluateDownstream('{{.Name}}')">Evaluate downstream</button>{{end}}<button onclick="clearOutputs('{{.ID}}')">Clear outputs</button><button onclick="moveCell('{{.ID}}',-1)">↑</button><button onclick="moveCell('{{.ID}}',1)">↓</button><button onclick="deleteCell('{{.ID}}')">Delete</button></div><div class="meta-row"><label>kind <select class="cell-kind"><option value="code" {{if eq .Kind "code"}}selected{{end}}>code</option><option value="markdown" {{if eq .Kind "markdown"}}selected{{end}}>markdown</option></select></label><label> name <input class="cell-name" value="{{.Name}}" placeholder="optional name"></label><label> depends-on <input class="cell-deps" value="{{join .DependsOn ","}}" placeholder="comma-separated names"></label></div><textarea onfocus="activeCell=this.closest('.cell').dataset.id" oninput="highlight(this)">{{.Source}}</textarea><pre class="highlight"></pre>{{if .Outputs}}<details class="outputs" open><summary>Out[{{.ExecutionCount}}] · {{len .Outputs}} output(s)</summary>{{range .Outputs}}<div class="output">{{if eq .Type "svg"}}{{.Source}}{{else if eq .Type "image"}}<img style="max-width:100%" src="data:{{.MIME}};base64,{{.Data}}">{{else if eq .Type "chart"}}<div class="chart" data-spec="{{.Spec}}"></div>{{else if eq .Type "diagram"}}<div class="diagram" data-renderer="{{.Renderer}}" data-source="{{.Source}}"></div>{{else if eq .Type "graph"}}<div class="graph" data-source="{{.Source}}"></div>{{else}}<pre class="{{if eq .Type "error"}}err{{end}}">{{.Text}}</pre>{{end}}</div>{{end}}</details>{{end}}</div>{{end}}</div>
<h2>Raw notebook</h2><pre id="raw"></pre>
<script>
var activeCell='', dirty=false
function setDirty(v){dirty=v;document.getElementById('notebook-dirty').textContent=v?'Unsaved changes':'Saved'}
function setTheme(t){if(t==='auto'){localStorage.removeItem('jokerNotebookTheme');document.documentElement.removeAttribute('data-theme')}else{localStorage.setItem('jokerNotebookTheme',t);document.documentElement.setAttribute('data-theme',t)}logMsg('Theme: '+t,false)}
(function(){var t=localStorage.getItem('jokerNotebookTheme');if(t)document.documentElement.setAttribute('data-theme',t)})()
function esc(s){return (s||'').replace(/[&<>]/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;'}[c]})}
function hi(s){return esc(s).replace(/"(?:\\.|[^"])*"/g,'<span class="str">$&</span>').replace(/\b(defn?|fn|let|letfn|loop|recur|if|do|quote|try|catch|throw|ns)\b/g,'<span class="kw">$1</span>').replace(/(:[\w!?*+<>=\/.-]+)/g,'<span class="sym">$1</span>')}
function highlight(t){t.nextElementSibling.innerHTML=hi(t.value);setDirty(true)}
document.querySelectorAll('textarea').forEach(function(t){t.nextElementSibling.innerHTML=hi(t.value)})
function loadStatus(){fetch('/api/status').then(r=>r.json()).then(s=>{document.getElementById('notebook-status').textContent=s.cellCount+' cells · '+s.outputCount+' outputs · '+Math.round(s.bytes/1024)+' KB'+(s.warning?' · WARNING: '+s.warning:'')})}
loadStatus()
function logMsg(s,isErr){var el=document.getElementById('notebook-log');el.textContent=s;el.className=isErr?'err':''}
function apiText(promise,ok){return promise.then(async r=>{var t=await r.text();if(!r.ok){logMsg(t||('HTTP '+r.status),true);throw new Error(t)};logMsg(ok||'OK',false);return t})}
function refresh(t){setDirty(false);document.getElementById('raw').textContent=t; setTimeout(function(){location.reload()},150)}
function splitDeps(s){return (s||'').split(',').map(x=>x.trim()).filter(Boolean)}
function sourcePayload(){return JSON.stringify({title:document.getElementById('notebook-title').value,cells:Array.from(document.querySelectorAll('.cell')).map(function(c){return {id:c.dataset.id,kind:c.querySelector('.cell-kind').value,name:c.querySelector('.cell-name').value.trim(),dependsOn:splitDeps(c.querySelector('.cell-deps').value),source:c.querySelector('textarea').value}})})}
function evaluateAll(){apiText(fetch('/api/evaluate-all',{method:'POST',headers:{'Content-Type':'application/json'},body:sourcePayload()}),'Evaluated all cells').then(refresh).catch(()=>{})}
function evaluateCell(id){var c=document.querySelector('.cell[data-id="'+CSS.escape(id)+'"]');var src=c?c.querySelector('textarea').value:'';apiText(fetch('/api/evaluate-cell?id='+encodeURIComponent(id),{method:'POST',headers:{'Content-Type':'text/plain'},body:src}),'Evaluated '+id).then(refresh).catch(()=>{})}
function saveNotebook(){apiText(fetch('/api/save-sources',{method:'POST',headers:{'Content-Type':'application/json'},body:sourcePayload()}),'Saved notebook').then(refresh).catch(()=>{})}
function exportMarkdown(){apiText(fetch('/api/save-sources',{method:'POST',headers:{'Content-Type':'application/json'},body:sourcePayload()}),'Saved before export').then(()=>apiText(fetch('/api/export/markdown'),'Exported Markdown')).then(t=>{document.getElementById('raw').textContent=t}).catch(()=>{})}
function loadRawEdn(){var raw=document.getElementById('raw').textContent.trim();if(!raw){alert('Paste notebook EDN into the raw pane first.');return}apiText(fetch('/api/save',{method:'POST',headers:{'Content-Type':'application/edn'},body:raw}),'Loaded raw EDN').then(refresh).catch(()=>{})}
function addCell(kind){apiText(fetch('/api/cell?kind='+encodeURIComponent(kind),{method:'POST'}),'Added '+kind+' cell').then(refresh).catch(()=>{})}
function deleteCell(id){if(confirm('Delete '+id+'?'))apiText(fetch('/api/cell?id='+encodeURIComponent(id),{method:'DELETE'}),'Deleted '+id).then(refresh).catch(()=>{})}
function clearOutputs(id){var url='/api/clear-outputs'+(id?'?id='+encodeURIComponent(id):'');apiText(fetch(url,{method:'POST'}),'Cleared outputs').then(refresh).catch(()=>{})}
function moveCell(id,delta){var ids=Array.from(document.querySelectorAll('.cell')).map(c=>c.dataset.id);var i=ids.indexOf(id),j=i+delta;if(i<0||j<0||j>=ids.length)return;var t=ids[i];ids[i]=ids[j];ids[j]=t;apiText(fetch('/api/reorder',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({ids:ids})}),'Reordered cells').then(refresh).catch(()=>{})}
function evaluateDownstream(name){apiText(fetch('/api/evaluate-downstream?name='+encodeURIComponent(name),{method:'POST',headers:{'Content-Type':'application/json'},body:sourcePayload()}),'Evaluated downstream of '+name).then(refresh).catch(()=>{})}
function checkDeps(){fetch('/api/dependencies').then(r=>r.json()).then(j=>alert(j.cycles&&j.cycles.length?'Dependency cycles: '+JSON.stringify(j.cycles):'No dependency cycles'))}
function showDependencyGraph(){fetch('/api/dependencies').then(r=>r.json()).then(j=>{var el=document.getElementById('dependency-graph');el.style.display='block';el.dataset.source=JSON.stringify(j.graph||{nodes:[],edges:[]});renderGraphs()})}
function parseMaybeJSON(s){try{return JSON.parse(s)}catch(e){return null}}
function renderCharts(){document.querySelectorAll('.chart').forEach(function(el){var spec=parseMaybeJSON(el.dataset.spec)||{};var data=(spec.series&&spec.series[0]&&spec.series[0].data)||spec.data||[];var labels=(spec.xAxis&&spec.xAxis.data)||data.map(function(_,i){return String(i+1)});var max=Math.max(1,...data.map(Number));var w=720,h=220,p=32,bw=(w-2*p)/Math.max(1,data.length);var svg='<svg viewBox="0 0 '+w+' '+h+'"><line class="axis" x1="'+p+'" y1="'+(h-p)+'" x2="'+(w-p)+'" y2="'+(h-p)+'"/>';data.forEach(function(v,i){var bh=(h-2*p)*Number(v)/max;var x=p+i*bw+3;var y=h-p-bh;svg+='<rect class="bar" x="'+x+'" y="'+y+'" width="'+Math.max(2,bw-6)+'" height="'+bh+'"><title>'+esc(labels[i])+': '+esc(String(v))+'</title></rect>'});svg+='</svg>';el.innerHTML=svg})}
function renderDiagrams(){document.querySelectorAll('.diagram').forEach(function(el){var r=el.dataset.renderer||'diagram';var s=el.dataset.source||'';el.innerHTML='<b>'+esc(r)+'</b><pre>'+esc(s)+'</pre>'})}
function renderGraphs(){document.querySelectorAll('.graph').forEach(function(el){var g=parseMaybeJSON(el.dataset.source)||{};var nodes=g.nodes||[],edges=g.edges||[];var w=720,h=260,cx=w/2,cy=h/2,rad=Math.min(w,h)/2-45;var pos={};nodes.forEach(function(n,i){var a=2*Math.PI*i/Math.max(1,nodes.length)-Math.PI/2;pos[n.id]={x:cx+rad*Math.cos(a),y:cy+rad*Math.sin(a),label:n.label||n.id}});var svg='<svg viewBox="0 0 '+w+' '+h+'"><defs><marker id="arrow" markerWidth="8" markerHeight="8" refX="8" refY="3" orient="auto"><path d="M0,0 L0,6 L8,3 z" fill="currentColor"/></marker></defs>';edges.forEach(function(e){var a=pos[e.from],b=pos[e.to];if(a&&b)svg+='<line class="edge" x1="'+a.x+'" y1="'+a.y+'" x2="'+b.x+'" y2="'+b.y+'"/>'});nodes.forEach(function(n){var p=pos[n.id];svg+='<circle class="node" cx="'+p.x+'" cy="'+p.y+'" r="22"/><text x="'+p.x+'" y="'+(p.y+4)+'" text-anchor="middle" font-size="11">'+esc(p.label)+'</text>'});el.innerHTML=svg+'</svg>'})}
document.addEventListener('input',function(e){if(e.target.matches('input,select,textarea'))setDirty(true)})
window.addEventListener('beforeunload',function(e){if(dirty){e.preventDefault();e.returnValue=''}})
document.addEventListener('keydown',function(e){if((e.ctrlKey||e.metaKey)&&e.key==='s'){e.preventDefault();saveNotebook()}else if((e.ctrlKey||e.metaKey)&&e.key==='Enter'){e.preventDefault();if(activeCell)evaluateCell(activeCell);else evaluateAll()}else if(e.shiftKey&&e.key==='Enter'){e.preventDefault();evaluateAll()}})
renderCharts();renderDiagrams();renderGraphs();
</script>
</body></html>`))

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
func BuildDependencyGraph(nb Notebook) DependencyGraph {
	graph := DependencyGraph{}
	seen := map[string]bool{}
	for _, c := range nb.Cells {
		id := c.Name
		if id == "" {
			id = c.ID
		}
		if !seen[id] {
			graph.Nodes = append(graph.Nodes, GraphNode{ID: id, Label: id})
			seen[id] = true
		}
		for _, dep := range c.DependsOn {
			if dep == "" {
				continue
			}
			if !seen[dep] {
				graph.Nodes = append(graph.Nodes, GraphNode{ID: dep, Label: dep})
				seen[dep] = true
			}
			graph.Edges = append(graph.Edges, GraphEdge{From: dep, To: id})
		}
	}
	return graph
}

func DependencyCycles(nb Notebook) [][]string {
	deps := map[string][]string{}
	for _, c := range nb.Cells {
		if c.Name != "" {
			deps[c.Name] = append([]string(nil), c.DependsOn...)
		}
	}
	var cycles [][]string
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var walk func(string, []string)
	walk = func(n string, stack []string) {
		if visiting[n] {
			for i, s := range stack {
				if s == n {
					cycles = append(cycles, append(append([]string(nil), stack[i:]...), n))
					break
				}
			}
			return
		}
		if visited[n] {
			return
		}
		visiting[n] = true
		for _, d := range deps[n] {
			if _, ok := deps[d]; ok {
				walk(d, append(stack, n))
			}
		}
		visiting[n] = false
		visited[n] = true
	}
	for n := range deps {
		walk(n, nil)
	}
	return cycles
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
