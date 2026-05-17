package main

import (
	corereader "github.com/rcarmo/go-joker/core/reader"
	"testing"
)

func TestDetectDialectFromFilename(t *testing.T) {
	tests := []struct {
		name string
		file string
		want corereader.Dialect
	}{
		{name: "edn", file: "deps.edn", want: corereader.EDNDialect},
		{name: "cljs", file: "src/app.cljs", want: corereader.CLJSDialect},
		{name: "joker", file: "script.joke", want: corereader.JokerDialect},
		{name: "default clj", file: "src/app.cljc", want: corereader.CLJDialect},
		{name: "extensionless default clj", file: "Makefile", want: corereader.CLJDialect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectDialect(tt.file); got != tt.want {
				t.Fatalf("detectDialect(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestDialectFromArg(t *testing.T) {
	tests := []struct {
		arg  string
		want corereader.Dialect
	}{
		{arg: "clj", want: corereader.CLJDialect},
		{arg: "corereader.CLJSDialect", want: corereader.CLJSDialect},
		{arg: "joker", want: corereader.JokerDialect},
		{arg: "EdN", want: corereader.EDNDialect},
		{arg: "unknown", want: corereader.UnknownDialect},
	}

	for _, tt := range tests {
		if got := dialectFromArg(tt.arg); got != tt.want {
			t.Fatalf("dialectFromArg(%q) = %v, want %v", tt.arg, got, tt.want)
		}
	}
}

func TestMatchesDialect(t *testing.T) {
	tests := []struct {
		path    string
		dialect corereader.Dialect
		want    bool
	}{
		{path: "src/app.clj", dialect: corereader.CLJDialect, want: true},
		{path: "src/app.cljs", dialect: corereader.CLJSDialect, want: true},
		{path: "src/app.joke", dialect: corereader.JokerDialect, want: true},
		{path: "deps.edn", dialect: corereader.EDNDialect, want: true},
		{path: "src/app.cljc", dialect: corereader.CLJDialect, want: false},
	}

	for _, tt := range tests {
		if got := matchesDialect(tt.path, tt.dialect); got != tt.want {
			t.Fatalf("matchesDialect(%q, %v) = %v, want %v", tt.path, tt.dialect, got, tt.want)
		}
	}
}
