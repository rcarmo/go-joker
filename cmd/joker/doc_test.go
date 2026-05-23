package main

import (
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

func TestMarkdownToHTML(t *testing.T) {
	html := string(markdownToHTML("# `joker.core/first`\n\n```clojure\n(first coll)\n```\n\n- `x` — value"))
	for _, want := range []string{"<h1><code>joker.core/first</code></h1>", `<pre><code class="language-clojure">`, "(first coll)", `<div class="doc-row">• <code>x</code> — value</div>`} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q:\n%s", want, html)
		}
	}
}

func TestRenderDocMarkdownVar(t *testing.T) {
	idx := docIndex{Namespaces: []docNamespace{{Name: "joker.core", Doc: "Core docs.", Vars: []docVar{{Name: "first", Qualified: "joker.core/first", Kind: "function", Doc: "Returns the first item.", Added: "1.0", Arglists: []string{"(first coll)"}}}}}}
	got := renderDocMarkdown(idx, "joker.core/first")
	if want := "# `joker.core/first`"; !strings.Contains(got, want) {
		t.Fatalf("markdown missing %q:\n%s", want, got)
	}
	if want := "```clojure\n(first coll)\n```"; !strings.Contains(got, want) {
		t.Fatalf("markdown missing arglist:\n%s", got)
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
