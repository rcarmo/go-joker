package main

import (
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"strings"

	. "github.com/rcarmo/go-joker/core"
	"github.com/rcarmo/go-joker/internal/notebook"
)

//go:embed notebook_assets/rich-demo.edn
var embeddedRichDemoNotebook string

func handleNotebookCommand(args []string) {
	if len(args) == 0 || args[0] == "serve" {
		serveNotebook(args)
		return
	}
	switch args[0] {
	case "new":
		newNotebook(args[1:])
	case "demo":
		demoNotebook(args[1:])
	case "run":
		runNotebook(args[1:])
	case "export":
		exportNotebook(args[1:])
	case "status":
		statusNotebook(args[1:])
	case "validate":
		validateNotebook(args[1:])
	case "deps":
		depsNotebook(args[1:])
	case "snapshots":
		snapshotsNotebook(args[1:])
	case "restore":
		restoreNotebook(args[1:])
	case "-h", "--help":
		printNotebookUsage()
	default:
		serveNotebook(args)
	}
}

func demoNotebook(args []string) {
	path := "rich-demo.edn"
	if len(args) > 0 {
		path = args[0]
	}
	if err := os.WriteFile(path, []byte(embeddedRichDemoNotebook), 0644); err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
}

func newNotebook(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(Stderr, "notebook new requires a file")
		os.Exit(2)
	}
	title := args[0]
	serve := false
	serveArgs := []string{args[0]}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if i+1 < len(args) {
				i++
				title = args[i]
			}
		case "--serve":
			serve = true
		case "--open", "--readonly", "--read-only", "-p", "--port", "--addr", "--token":
			serveArgs = append(serveArgs, args[i])
			if (args[i] == "-p" || args[i] == "--port" || args[i] == "--addr" || args[i] == "--token") && i+1 < len(args) {
				i++
				serveArgs = append(serveArgs, args[i])
			}
		}
	}
	nb := notebook.New(title)
	nb.Cells = []notebook.Cell{
		{ID: "cell-1", Kind: "markdown", Source: "# " + title, State: "idle"},
		{ID: "cell-2", Kind: "code", Source: "(+ 1 2)", State: "idle"},
	}
	if err := notebook.Save(args[0], nb); err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
	if serve {
		serveNotebook(serveArgs)
	}
}

func serveNotebook(args []string) {
	addr := "127.0.0.1:8080"
	path := "notebook.edn"
	open := false
	readOnly := false
	token := ""
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p", "--port":
			i++
			if i >= len(args) {
				fmt.Fprintln(Stderr, "notebook: -p requires a port")
				os.Exit(2)
			}
			p, err := parseNotebookPort(args[i])
			if err != nil {
				fmt.Fprintln(Stderr, err)
				os.Exit(2)
			}
			addr = "127.0.0.1:" + p
		case "--addr":
			i++
			if i >= len(args) {
				fmt.Fprintln(Stderr, "notebook: --addr requires host:port")
				os.Exit(2)
			}
			addr = args[i]
		case "--open":
			open = true
		case "--readonly", "--read-only":
			readOnly = true
		case "--token":
			i++
			if i >= len(args) {
				fmt.Fprintln(Stderr, "notebook: --token requires a value")
				os.Exit(2)
			}
			token = args[i]
		default:
			if !strings.HasPrefix(args[i], "-") {
				path = args[i]
			}
		}
	}
	notebook.AuthToken = token
	notebook.ReadOnly = readOnly
	if !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "localhost:") {
		fmt.Fprintf(Stderr, "WARNING: notebook server bound to %s exposes trusted local code execution.\n", addr)
		if token == "" {
			fmt.Fprintln(Stderr, "WARNING: consider --token for non-local notebook binds.")
		}
	}
	if err := notebook.Serve(addr, path, open); err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
}

func restoreNotebook(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(Stderr, "notebook restore requires a file and snapshot path")
		os.Exit(2)
	}
	if _, err := notebook.RestoreSnapshot(args[0], args[1]); err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
}

func snapshotsNotebook(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(Stderr, "notebook snapshots requires a file")
		os.Exit(2)
	}
	snaps, err := notebook.ListSnapshots(args[0])
	if err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
	fmt.Fprint(Stdout, "[")
	for i, snap := range snaps {
		if i > 0 {
			fmt.Fprint(Stdout, ",")
		}
		fmt.Fprintf(Stdout, "{\"path\":%q,\"size\":%d}", snap.Path, snap.Size)
	}
	fmt.Fprintln(Stdout, "]")
}

func validateNotebook(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(Stderr, "notebook validate requires a file")
		os.Exit(2)
	}
	nb, err := notebook.Load(args[0])
	if err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
	if cycles := notebook.DependencyCycles(nb); len(cycles) > 0 {
		fmt.Fprintf(Stderr, "notebook dependency cycles: %v\n", cycles)
		os.Exit(1)
	}
	fmt.Fprintf(Stdout, "notebook ok: %s (%d cells)\n", args[0], len(nb.Cells))
}

func depsNotebook(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(Stderr, "notebook deps requires a file")
		os.Exit(2)
	}
	nb, err := notebook.Load(args[0])
	if err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
	graph := notebook.BuildDependencyGraph(nb)
	cycles := notebook.DependencyCycles(nb)
	fmt.Fprintf(Stdout, "{\"nodes\":[")
	for i, n := range graph.Nodes {
		if i > 0 {
			fmt.Fprint(Stdout, ",")
		}
		fmt.Fprintf(Stdout, "{\"id\":%q,\"label\":%q}", n.ID, n.Label)
	}
	fmt.Fprintf(Stdout, "],\"edges\":[")
	for i, e := range graph.Edges {
		if i > 0 {
			fmt.Fprint(Stdout, ",")
		}
		fmt.Fprintf(Stdout, "{\"from\":%q,\"to\":%q}", e.From, e.To)
	}
	fmt.Fprintf(Stdout, "],\"cycles\":[")
	for i, cycle := range cycles {
		if i > 0 {
			fmt.Fprint(Stdout, ",")
		}
		fmt.Fprint(Stdout, "[")
		for j, node := range cycle {
			if j > 0 {
				fmt.Fprint(Stdout, ",")
			}
			fmt.Fprintf(Stdout, "%q", node)
		}
		fmt.Fprint(Stdout, "]")
	}
	fmt.Fprintln(Stdout, "]}")
}

func statusNotebook(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(Stderr, "notebook status requires a file")
		os.Exit(2)
	}
	nb, err := notebook.Load(args[0])
	if err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
	status := notebook.BuildStatus(nb)
	fmt.Fprintf(Stdout, "{\"title\":%q,\"cellCount\":%d,\"outputCount\":%d,\"bytes\":%d", status.Title, status.CellCount, status.OutputCount, status.Bytes)
	if status.Warning != "" {
		fmt.Fprintf(Stdout, ",\"warning\":%q", status.Warning)
	}
	fmt.Fprintln(Stdout, "}")
}

func runNotebook(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(Stderr, "notebook run requires a file")
		os.Exit(2)
	}
	path := ""
	noSave := false
	summary := false
	for _, arg := range args {
		switch arg {
		case "--no-save":
			noSave = true
		case "--summary", "--json-summary":
			summary = true
		default:
			if !strings.HasPrefix(arg, "-") && path == "" {
				path = arg
			}
		}
	}
	if path == "" {
		fmt.Fprintln(Stderr, "notebook run requires a file")
		os.Exit(2)
	}
	nb, err := notebook.Load(path)
	if err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
	notebook.Run(&nb)
	if summary {
		writeNotebookRunSummary(nb)
	}
	if noSave {
		return
	}
	if err := notebook.Save(path, nb); err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
}

func writeNotebookRunSummary(nb notebook.Notebook) {
	status := notebook.BuildStatus(nb)
	fmt.Fprintf(Stdout, "{\"title\":%q,\"cellCount\":%d,\"outputCount\":%d,\"cells\":[", status.Title, status.CellCount, status.OutputCount)
	for i, c := range nb.Cells {
		if i > 0 {
			fmt.Fprint(Stdout, ",")
		}
		fmt.Fprintf(Stdout, "{\"id\":%q,\"kind\":%q,\"name\":%q,\"state\":%q,\"outputs\":%d}", c.ID, c.Kind, c.Name, c.State, len(c.Outputs))
	}
	fmt.Fprintln(Stdout, "]}")
}

func exportNotebook(args []string) {
	out := ""
	file := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			i++
			if i < len(args) {
				out = args[i]
			}
		case "--format":
			i++
		default:
			if file == "" {
				file = args[i]
			}
		}
	}
	if file == "" {
		fmt.Fprintln(Stderr, "notebook export requires a file")
		os.Exit(2)
	}
	nb, err := notebook.Load(file)
	if err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
	if out == "" {
		if err := notebook.ExportMarkdown(Stdout, nb); err != nil {
			fmt.Fprintln(Stderr, err)
			os.Exit(1)
		}
		return
	}
	f, err := os.Create(out)
	if err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := notebook.ExportMarkdown(f, nb); err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
}

func parseNotebookPort(raw string) (string, error) {
	p, err := strconv.Atoi(raw)
	if err != nil || p <= 0 || p > 65535 {
		return "", fmt.Errorf("notebook: invalid port %q", raw)
	}
	return strconv.Itoa(p), nil
}

func printNotebookUsage() {
	fmt.Fprintln(Stdout, `Usage:
  joker notebook [file.edn] [-p 8080] [--open] [--token secret] [--readonly]
  joker notebook serve [file.edn] [-p 8080] [--token secret] [--readonly]
  joker notebook new file.edn [--title "Title"] [--serve] [--open] [--token secret] [--readonly]
  joker notebook demo [file.edn]
  joker notebook run file.edn [--no-save] [--summary]
  joker notebook validate file.edn
  joker notebook status file.edn
  joker notebook deps file.edn
  joker notebook snapshots file.edn
  joker notebook restore file.edn snapshot.bak.edn
  joker notebook export file.edn [-o report.md]`)
}
