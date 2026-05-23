package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"github.com/yuin/goldmark"
)

type docIndex struct {
	Namespaces []docNamespace `json:"namespaces"`
}

type docNamespace struct {
	Name string   `json:"name"`
	Doc  string   `json:"doc,omitempty"`
	Vars []docVar `json:"vars"`
}

type docVar struct {
	Name      string   `json:"name"`
	Qualified string   `json:"qualified"`
	Kind      string   `json:"kind"`
	Doc       string   `json:"doc,omitempty"`
	Added     string   `json:"added,omitempty"`
	Arglists  []string `json:"arglists,omitempty"`
}

func handleDocCommand(args []string) {
	format := "markdown"
	addr := "127.0.0.1:8080"
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 >= len(args) {
				fmt.Fprintln(Stderr, "doc: --format requires markdown or json")
				os.Exit(2)
			}
			i++
			format = args[i]
		case "--json":
			format = "json"
		case "--addr":
			if i+1 >= len(args) {
				fmt.Fprintln(Stderr, "doc: --addr requires host:port")
				os.Exit(2)
			}
			i++
			addr = args[i]
		case "-p", "--port":
			if i+1 >= len(args) {
				fmt.Fprintln(Stderr, "doc: -p/--port requires a port number")
				os.Exit(2)
			}
			i++
			port, err := parseDocPort(args[i])
			if err != nil {
				fmt.Fprintln(Stderr, err)
				os.Exit(2)
			}
			addr = "127.0.0.1:" + port
		case "-h", "--help":
			printDocUsage()
			return
		default:
			filtered = append(filtered, args[i])
		}
	}

	idx := buildDocIndex()
	if len(filtered) > 0 && filtered[0] == "serve" {
		if err := serveDocs(addr, idx); err != nil {
			fmt.Fprintln(Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(filtered) > 0 && filtered[0] == "search" {
		filtered = filtered[1:]
	}
	query := strings.Join(filtered, " ")
	if format == "json" {
		payload, err := json.MarshalIndent(queryDocs(idx, query), "", "  ")
		if err != nil {
			fmt.Fprintln(Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(Stdout, string(payload))
		return
	}
	if format != "markdown" {
		fmt.Fprintln(Stderr, "doc: --format must be markdown or json")
		os.Exit(2)
	}
	fmt.Fprint(Stdout, renderDocMarkdown(idx, query))
}

func printDocUsage() {
	fmt.Fprintln(Stdout, `Usage:
  joker doc [symbol-or-namespace]
  joker doc search QUERY
  joker doc --format json [symbol-or-namespace]
  joker doc serve [-p 8080]
  joker doc serve [--addr 127.0.0.1:8080]

Examples:
  joker doc joker.core/first
  joker doc joker.string
  joker doc search websocket`)
}

func parseDocPort(raw string) (string, error) {
	port, err := strconv.Atoi(raw)
	if err != nil || port <= 0 || port > 65535 {
		return "", fmt.Errorf("doc: invalid port %q", raw)
	}
	return strconv.Itoa(port), nil
}

func buildDocIndex() docIndex {
	idx := docIndex{}
	for _, ns := range GLOBAL_ENV.Namespaces {
		if ns == nil || ns.Name.Name() == "user" {
			continue
		}
		ns.MaybeLazy("doc index")
		dn := docNamespace{Name: ns.Name.Name(), Doc: metaString(ns.GetMeta(), "doc")}
		for _, vr := range ns.Mappings() {
			if vr == nil || metaBool(vr.GetMeta(), "private") {
				continue
			}
			name := varShortName(vr.Name())
			dn.Vars = append(dn.Vars, docVar{
				Name:      name,
				Qualified: vr.Name(),
				Kind:      docKind(vr),
				Doc:       metaString(vr.GetMeta(), "doc"),
				Added:     metaString(vr.GetMeta(), "added"),
				Arglists:  metaArglists(vr.GetMeta(), name),
			})
		}
		sort.Slice(dn.Vars, func(i, j int) bool { return dn.Vars[i].Name < dn.Vars[j].Name })
		idx.Namespaces = append(idx.Namespaces, dn)
	}
	sort.Slice(idx.Namespaces, func(i, j int) bool { return idx.Namespaces[i].Name < idx.Namespaces[j].Name })
	return idx
}

func varShortName(q string) string {
	if i := strings.LastIndex(q, "/"); i >= 0 {
		return q[i+1:]
	}
	return q
}

func metaString(m coretypes.Map, key string) string {
	if m == nil {
		return ""
	}
	ok, v := m.Get(coretypes.MakeKeyword(STRINGS.Intern, key))
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(coretypes.String); ok {
		return s.S
	}
	return v.ToString(false)
}

func metaBool(m coretypes.Map, key string) bool {
	if m == nil {
		return false
	}
	ok, v := m.Get(coretypes.MakeKeyword(STRINGS.Intern, key))
	if !ok {
		return false
	}
	b, ok := v.(coretypes.Boolean)
	return ok && b.B
}

func metaArglists(m coretypes.Map, name string) []string {
	if m == nil {
		return nil
	}
	ok, v := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "arglists"))
	if !ok || v == nil {
		return nil
	}
	seqable, ok := v.(coretypes.Seqable)
	if !ok {
		return nil
	}
	var out []string
	for it := seqable.Seq(); it != nil && !it.IsEmpty(); it = it.Rest() {
		out = append(out, fmt.Sprintf("(%s %s)", name, strings.Trim(it.First().ToString(false), "[]")))
	}
	return out
}

func docKind(vr *Var) string {
	v := vr.Resolve()
	if _, ok := v.(coretypes.Callable); ok {
		return "function"
	}
	return "value"
}

type docQueryResult struct {
	Query      string         `json:"query,omitempty"`
	Namespaces []docNamespace `json:"namespaces,omitempty"`
	Vars       []docVar       `json:"vars,omitempty"`
}

func queryDocs(idx docIndex, query string) docQueryResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return docQueryResult{Namespaces: idx.Namespaces}
	}
	q := strings.ToLower(query)
	res := docQueryResult{Query: query}
	seenVars := map[string]bool{}
	for _, ns := range idx.Namespaces {
		if strings.EqualFold(ns.Name, query) {
			res.Namespaces = append(res.Namespaces, ns)
			return res
		}
		if strings.Contains(strings.ToLower(ns.Name), q) || strings.Contains(strings.ToLower(ns.Doc), q) {
			res.Namespaces = append(res.Namespaces, ns)
		}
		for _, v := range ns.Vars {
			if seenVars[v.Qualified] {
				continue
			}
			if strings.EqualFold(v.Qualified, query) || strings.EqualFold(v.Name, query) || strings.Contains(strings.ToLower(v.Qualified), q) || strings.Contains(strings.ToLower(v.Doc), q) {
				seenVars[v.Qualified] = true
				res.Vars = append(res.Vars, v)
			}
		}
	}
	return res
}

func renderDocMarkdown(idx docIndex, query string) string {
	res := queryDocs(idx, query)
	var b strings.Builder
	if strings.TrimSpace(query) == "" {
		b.WriteString("# Joker runtime documentation\n\n## Namespaces\n\n")
		for _, ns := range res.Namespaces {
			b.WriteString(fmt.Sprintf("- %s", docSymbolLink(ns.Name, ns.Name)))
			if ns.Doc != "" {
				b.WriteString(" — " + firstLine(ns.Doc))
			}
			b.WriteString("\n")
		}
		return b.String()
	}
	if len(res.Namespaces) == 1 && strings.EqualFold(res.Namespaces[0].Name, query) {
		ns := res.Namespaces[0]
		b.WriteString(fmt.Sprintf("# %s\n\n%s\n\n## Vars\n\n", docSymbolLink(ns.Name, ns.Name), ns.Doc))
		for _, v := range ns.Vars {
			renderVarSummary(&b, v)
		}
		return b.String()
	}
	if len(res.Vars) == 1 && (strings.EqualFold(res.Vars[0].Qualified, query) || strings.EqualFold(res.Vars[0].Name, query)) {
		renderVarFull(&b, res.Vars[0])
		return b.String()
	}
	b.WriteString(fmt.Sprintf("# Matches for `%s`\n\n", query))
	for _, ns := range res.Namespaces {
		b.WriteString(fmt.Sprintf("- namespace %s — %s\n", docSymbolLink(ns.Name, ns.Name), firstLine(ns.Doc)))
	}
	for _, v := range res.Vars {
		b.WriteString(fmt.Sprintf("- %s — %s\n", docSymbolLink(v.Qualified, v.Qualified), firstLine(v.Doc)))
	}
	if len(res.Namespaces) == 0 && len(res.Vars) == 0 {
		b.WriteString("No matches.\n")
	}
	return b.String()
}

func renderVarSummary(b *strings.Builder, v docVar) {
	b.WriteString(fmt.Sprintf("### %s\n\n", docSymbolLink(v.Name, v.Qualified)))
	if len(v.Arglists) > 0 {
		b.WriteString("```clojure\n" + strings.Join(v.Arglists, "\n") + "\n```\n\n")
	}
	if v.Doc != "" {
		b.WriteString(firstLine(v.Doc) + "\n\n")
	}
}

func renderVarFull(b *strings.Builder, v docVar) {
	b.WriteString(fmt.Sprintf("# %s\n\n", docSymbolLink(v.Qualified, v.Qualified)))
	if len(v.Arglists) > 0 {
		b.WriteString("```clojure\n" + strings.Join(v.Arglists, "\n") + "\n```\n\n")
	}
	if v.Doc != "" {
		b.WriteString(v.Doc + "\n\n")
	}
	ns := strings.Split(v.Qualified, "/")[0]
	b.WriteString(fmt.Sprintf("Namespace: %s  \nKind: `%s`", docSymbolLink(ns, ns), v.Kind))
	if v.Added != "" {
		b.WriteString(fmt.Sprintf("  \nAdded: `%s`", v.Added))
	}
	b.WriteString("\n")
}

func docSymbolLink(label string, query string) string {
	return fmt.Sprintf("[`%s`](?q=%s)", label, url.QueryEscape(query))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func renderDocHTML(markdown string) (template.HTML, error) {
	var b bytes.Buffer
	if err := goldmark.Convert([]byte(markdown), &b); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

func serveDocs(addr string, idx docIndex) error {
	fmt.Fprintf(Stdout, "Serving Joker docs at http://%s/\n", addr)
	return http.ListenAndServe(addr, docsHandler(idx))
}

func docsHandler(idx docIndex) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		page, err := renderDocHTML(renderDocMarkdown(idx, q))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = docsPage.Execute(w, struct {
			Query string
			HTML  template.HTML
		}{q, page})
	})
	mux.HandleFunc("/api/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(queryDocs(idx, r.URL.Query().Get("q")))
	})
	return mux
}

var docsPage = template.Must(template.New("docs").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Joker docs</title><style>:root{--font-ui:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif;--font-mono:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono",monospace}body{font-family:var(--font-ui);max-width:1100px;margin:2rem auto;padding:0 1rem;line-height:1.45}form{display:flex;gap:.5rem;position:sticky;top:0;background:Canvas;padding:.5rem 0}input{flex:1;padding:.5rem;font-family:var(--font-ui)}button{padding:.5rem;font-family:var(--font-ui)}.doc{margin-top:1rem}.doc pre{background:#f3f4f6;padding:1rem;overflow:auto;border-radius:6px}.doc code{background:#f3f4f6;padding:.1rem .25rem;border-radius:4px;font-family:var(--font-mono)}.doc pre code{padding:0}h1,h2,h3{line-height:1.2}@media(prefers-color-scheme:dark){body{background:#0d1117;color:#e6edf3}form{background:#0d1117}.doc pre,.doc code{background:#161b22}}</style></head><body><form><input name="q" value="{{.Query}}" placeholder="namespace, symbol, or search text"><button>Search</button></form><main class="doc">{{.HTML}}</main></body></html>`))
