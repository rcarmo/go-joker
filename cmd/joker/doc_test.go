package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestUsageMentionsNotebookCommands(t *testing.T) {
	var b strings.Builder
	usage(&b)
	for _, want := range []string{"notebook status", "notebook deps", "notebook snapshots", "notebook restore"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("usage missing %q:\n%s", want, b.String())
		}
	}
}

func TestRenderDocHTML(t *testing.T) {
	html, err := renderDocHTML("# `joker.core/first`\n\n```clojure\n(first coll)\n```\n\n- `x` — value")
	if err != nil {
		t.Fatal(err)
	}
	got := string(html)
	for _, want := range []string{"<h1><code>joker.core/first</code></h1>", `<pre><code class="language-clojure">`, "(first coll)", "<li><code>x</code> — value</li>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("html missing %q:\n%s", want, got)
		}
	}
}

func TestRenderDocMarkdownVar(t *testing.T) {
	idx := docIndex{Namespaces: []docNamespace{{Name: "joker.core", Doc: "Core docs.", Vars: []docVar{{Name: "first", Qualified: "joker.core/first", Kind: "function", Doc: "Returns the first item.", Added: "1.0", Arglists: []string{"(first coll)"}}}}}}
	got := renderDocMarkdown(idx, "joker.core/first")
	if want := "# [`joker.core/first`](?q=joker.core%2Ffirst)"; !strings.Contains(got, want) {
		t.Fatalf("markdown missing %q:\n%s", want, got)
	}
	if want := "```clojure\n(first coll)\n```"; !strings.Contains(got, want) {
		t.Fatalf("markdown missing arglist:\n%s", got)
	}
}

func TestRenderDocMarkdownLinksSymbols(t *testing.T) {
	idx := docIndex{Namespaces: []docNamespace{{Name: "joker.core", Doc: "Core docs.", Vars: []docVar{{Name: "first", Qualified: "joker.core/first", Kind: "function", Doc: "Returns the first item.", Arglists: []string{"(first coll)"}}}}}}
	for name, tt := range map[string]struct {
		got  string
		want []string
	}{
		"index":     {got: renderDocMarkdown(idx, ""), want: []string{"[`joker.core`](?q=joker.core)"}},
		"namespace": {got: renderDocMarkdown(idx, "joker.core"), want: []string{"[`joker.core`](?q=joker.core)", "[`first`](?q=joker.core%2Ffirst)"}},
		"search":    {got: renderDocMarkdown(idx, "first"), want: []string{"[`joker.core/first`](?q=joker.core%2Ffirst)"}},
	} {
		for _, want := range tt.want {
			if !strings.Contains(tt.got, want) {
				t.Fatalf("%s markdown missing %q:\n%s", name, want, tt.got)
			}
		}
	}
}

func TestRenderDocHTMLVar(t *testing.T) {
	html, err := renderDocHTML("# [`joker.core/first`](?q=joker.core%2Ffirst)\n\n```clojure\n(first coll)\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	got := string(html)
	for _, want := range []string{`<h1><a href="?q=joker.core%2Ffirst"><code>joker.core/first</code></a></h1>`, `<pre><code class="language-clojure">`} {
		if !strings.Contains(got, want) {
			t.Fatalf("html missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "# `joker.core/first`") {
		t.Fatalf("html still contains raw markdown:\n%s", got)
	}
}

func TestDocsHandlerRendersHTML(t *testing.T) {
	idx := docIndex{Namespaces: []docNamespace{{Name: "joker.core", Doc: "Core docs.", Vars: []docVar{{Name: "first", Qualified: "joker.core/first", Kind: "function", Doc: "Returns the first item.", Added: "1.0", Arglists: []string{"(first coll)"}}}}}}
	w := httptest.NewRecorder()
	docsHandler(idx).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?q=joker.core/first", nil))
	got := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("docs handler code=%d content-type=%s body=%s", w.Code, w.Header().Get("Content-Type"), got)
	}
	if !strings.Contains(got, `<h1><a href="?q=joker.core%2Ffirst"><code>joker.core/first</code></a></h1>`) || strings.Contains(got, "# `joker.core/first`") {
		t.Fatalf("docs handler did not render markdown as HTML:\n%s", got)
	}
	for _, want := range []string{"--font-ui:ui-sans-serif", "--font-mono:ui-monospace", "font-family:var(--font-ui)", "font-family:var(--font-mono)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("docs handler missing font stack %q:\n%s", want, got)
		}
	}
}

func TestParseDocPort(t *testing.T) {
	got, err := parseDocPort("9090")
	if err != nil || got != "9090" {
		t.Fatalf("parseDocPort = %q, %v", got, err)
	}
	for _, raw := range []string{"0", "65536", "abc"} {
		if _, err := parseDocPort(raw); err == nil {
			t.Fatalf("parseDocPort(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestNotebookServeWarnsForNonLocalhost(t *testing.T) {
	// Keep this as a source-level smoke check: serveNotebook itself blocks in ListenAndServe.
	// The warning is emitted before Serve is called when --addr is not localhost.
	if !strings.Contains(readNotebookSourceForTest(), "exposes trusted local code execution") {
		t.Fatal("notebook non-localhost warning missing")
	}
}

func readNotebookSourceForTest() string {
	data, _ := os.ReadFile("notebook.go")
	return string(data)
}

func TestParseNotebookPort(t *testing.T) {
	got, err := parseNotebookPort("8081")
	if err != nil || got != "8081" {
		t.Fatalf("parseNotebookPort = %q, %v", got, err)
	}
	if _, err := parseNotebookPort("70000"); err == nil {
		t.Fatal("parseNotebookPort accepted invalid port")
	}
}

func TestQueryDocsNamespaceAndSearch(t *testing.T) {
	idx := docIndex{Namespaces: []docNamespace{{Name: "joker.http", Doc: "HTTP and websocket helpers.", Vars: []docVar{{Name: "server", Qualified: "joker.http/server", Doc: "Starts a server."}}}}}
	if got := queryDocs(idx, "joker.http"); len(got.Namespaces) != 1 || got.Namespaces[0].Name != "joker.http" {
		t.Fatalf("namespace query = %#v", got)
	}
	if got := queryDocs(idx, "websocket"); len(got.Namespaces) != 1 {
		t.Fatalf("search query = %#v", got)
	}
}
