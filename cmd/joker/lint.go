package main

import (
	"fmt"
	corereader "github.com/rcarmo/go-joker/core/reader"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	"os"
	"path/filepath"
	"strings"

	. "github.com/rcarmo/go-joker/core"
)

func makeDialectKeyword(dialect corereader.Dialect) coretypes.Keyword {
	switch dialect {
	case corereader.EDNDialect:
		return coretypes.MakeKeyword(STRINGS.Intern, "clj")
	case corereader.CLJDialect:
		return coretypes.MakeKeyword(STRINGS.Intern, "clj")
	case corereader.CLJSDialect:
		return coretypes.MakeKeyword(STRINGS.Intern, "cljs")
	default:
		return coretypes.MakeKeyword(STRINGS.Intern, "joker")
	}
}

func configureLinterMode(dialect corereader.Dialect, filename string, workingDir string) {
	ProcessLinterData(dialect)
	ProcessLinterFiles(dialect, filename, workingDir)
	if dialect != corereader.JokerDialect {
		RemoveJokerNamespaces()
	}
	GLOBAL_ENV.CoreNamespace.Resolve("*loaded-libs*").Value = corecollections.EmptySet()
	LINTER_MODE = true
	DIALECT = dialect
	lm, _ := GLOBAL_ENV.Resolve(coretypes.MakeSymbol(STRINGS.Intern, "joker.core/*linter-mode*"))
	lm.Value = coretypes.Boolean{B: true}
	GLOBAL_ENV.Features = GLOBAL_ENV.Features.Disjoin(coretypes.MakeKeyword(STRINGS.Intern, "joker")).Conj(makeDialectKeyword(dialect)).(coretypes.Set)
	EnableIdentValidation()
}

func detectDialect(filename string) corereader.Dialect {
	switch {
	case strings.HasSuffix(filename, ".edn"):
		return corereader.EDNDialect
	case strings.HasSuffix(filename, ".cljs"):
		return corereader.CLJSDialect
	case strings.HasSuffix(filename, ".joke"):
		return corereader.JokerDialect
	}
	return corereader.CLJDialect
}

func lintFile(filename string, dialect corereader.Dialect, workingDir string) {
	phase := corereader.ParsePhase
	if dialect == corereader.EDNDialect {
		phase = corereader.ReadPhase
	}
	ReadConfig(filename, workingDir)
	configureLinterMode(dialect, filename, workingDir)
	if processFile(filename, phase) == nil {
		WarnOnUnusedNamespaces()
		WarnOnUnusedVars()
	}
}

func matchesDialect(path string, dialect corereader.Dialect) bool {
	ext := ".clj"
	switch dialect {
	case corereader.CLJSDialect:
		ext = ".cljs"
	case corereader.JokerDialect:
		ext = ".joke"
	case corereader.EDNDialect:
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

func lintDir(dirname string, dialect corereader.Dialect, reportGloballyUnused bool) {
	var processErr error
	phase := corereader.ParsePhase
	if dialect == corereader.EDNDialect {
		phase = corereader.ReadPhase
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
			GLOBAL_ENV.CoreNamespace.Resolve("*loaded-libs*").Value = corecollections.EmptySet()
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

func dialectFromArg(arg string) corereader.Dialect {
	switch strings.ToLower(arg) {
	case "clj":
		return corereader.CLJDialect
	case "cljs":
		return corereader.CLJSDialect
	case "joker":
		return corereader.JokerDialect
	case "edn":
		return corereader.EDNDialect
	}
	return corereader.UnknownDialect
}
