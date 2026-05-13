package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/rcarmo/go-joker/core"
)

func makeDialectKeyword(dialect Dialect) Keyword {
	switch dialect {
	case EDN:
		return MakeKeyword("clj")
	case CLJ:
		return MakeKeyword("clj")
	case CLJS:
		return MakeKeyword("cljs")
	default:
		return MakeKeyword("joker")
	}
}

func configureLinterMode(dialect Dialect, filename string, workingDir string) {
	ProcessLinterData(dialect)
	ProcessLinterFiles(dialect, filename, workingDir)
	if dialect != JOKER {
		RemoveJokerNamespaces()
	}
	GLOBAL_ENV.CoreNamespace.Resolve("*loaded-libs*").Value = EmptySet()
	LINTER_MODE = true
	DIALECT = dialect
	lm, _ := GLOBAL_ENV.Resolve(MakeSymbol("joker.core/*linter-mode*"))
	lm.Value = Boolean{B: true}
	GLOBAL_ENV.Features = GLOBAL_ENV.Features.Disjoin(MakeKeyword("joker")).Conj(makeDialectKeyword(dialect)).(Set)
	EnableIdentValidation()
}

func detectDialect(filename string) Dialect {
	switch {
	case strings.HasSuffix(filename, ".edn"):
		return EDN
	case strings.HasSuffix(filename, ".cljs"):
		return CLJS
	case strings.HasSuffix(filename, ".joke"):
		return JOKER
	}
	return CLJ
}

func lintFile(filename string, dialect Dialect, workingDir string) {
	phase := PARSE
	if dialect == EDN {
		phase = READ
	}
	ReadConfig(filename, workingDir)
	configureLinterMode(dialect, filename, workingDir)
	if processFile(filename, phase) == nil {
		WarnOnUnusedNamespaces()
		WarnOnUnusedVars()
	}
}

func matchesDialect(path string, dialect Dialect) bool {
	ext := ".clj"
	switch dialect {
	case CLJS:
		ext = ".cljs"
	case JOKER:
		ext = ".joke"
	case EDN:
		ext = ".edn"
	}
	return strings.HasSuffix(path, ext)
}

func isIgnored(path string) bool {
	for _, r := range WARNINGS.IgnoredFileRegexes {
		m := r.FindStringSubmatchIndex(path)
		if len(m) > 0 {
			if m[1]-m[0] == len(path) {
				return true
			}
		}
	}
	return false
}

func lintDir(dirname string, dialect Dialect, reportGloballyUnused bool) {
	var processErr error
	phase := PARSE
	if dialect == EDN {
		phase = READ
	}
	ns := GLOBAL_ENV.CurrentNamespace()
	ReadConfig("", dirname)
	configureLinterMode(dialect, "", dirname)
	filepath.Walk(dirname, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintln(Stderr, "Error: ", err)
			return nil
		}
		if !info.IsDir() && matchesDialect(path, dialect) && !isIgnored(path) {
			GLOBAL_ENV.CoreNamespace.Resolve("*loaded-libs*").Value = EmptySet()
			processErr = processFile(path, phase)
			if processErr == nil {
				WarnOnUnusedNamespaces()
				WarnOnUnusedVars()
			}
			ResetUsage()
			GLOBAL_ENV.SetCurrentNamespace(ns)
		}
		return nil
	})
	if processErr == nil && reportGloballyUnused {
		WarnOnGloballyUnusedNamespaces()
		WarnOnGloballyUnusedVars()
	}
}

func dialectFromArg(arg string) Dialect {
	switch strings.ToLower(arg) {
	case "clj":
		return CLJ
	case "cljs":
		return CLJS
	case "joker":
		return JOKER
	case "edn":
		return EDN
	}
	return UNKNOWN
}
