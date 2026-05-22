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

func TestQueryDocsNamespaceAndSearch(t *testing.T) {
	idx := docIndex{Namespaces: []docNamespace{{Name: "joker.http", Doc: "HTTP and websocket helpers.", Vars: []docVar{{Name: "server", Qualified: "joker.http/server", Doc: "Starts a server."}}}}}
	if got := queryDocs(idx, "joker.http"); len(got.Namespaces) != 1 || got.Namespaces[0].Name != "joker.http" {
		t.Fatalf("namespace query = %#v", got)
	}
	if got := queryDocs(idx, "websocket"); len(got.Namespaces) != 1 {
		t.Fatalf("search query = %#v", got)
	}
}
