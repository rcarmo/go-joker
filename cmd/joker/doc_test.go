package main

import (
	"strings"
	"testing"
)

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
