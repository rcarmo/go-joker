package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	. "github.com/rcarmo/go-joker/core"
	"github.com/rcarmo/go-joker/internal/notebook"
)

func handleNotebookCommand(args []string) {
	if len(args) == 0 || args[0] == "serve" {
		serveNotebook(args)
		return
	}
	switch args[0] {
	case "run":
		if len(args) < 2 {
			fmt.Fprintln(Stderr, "notebook run requires a file")
			os.Exit(2)
		}
		nb, err := notebook.Load(args[1])
		if err != nil {
			fmt.Fprintln(Stderr, err)
			os.Exit(1)
		}
		notebook.Run(&nb)
		if err := notebook.Save(args[1], nb); err != nil {
			fmt.Fprintln(Stderr, err)
			os.Exit(1)
		}
	case "export":
		exportNotebook(args[1:])
	case "status":
		statusNotebook(args[1:])
	case "deps":
		depsNotebook(args[1:])
	case "-h", "--help":
		printNotebookUsage()
	default:
		serveNotebook(args)
	}
}

func serveNotebook(args []string) {
	addr := "127.0.0.1:8080"
	path := "notebook.edn"
	open := false
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
		default:
			if !strings.HasPrefix(args[i], "-") {
				path = args[i]
			}
		}
	}
	if err := notebook.Serve(addr, path, open); err != nil {
		fmt.Fprintln(Stderr, err)
		os.Exit(1)
	}
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
  joker notebook [file.edn] [-p 8080] [--open]
  joker notebook serve [file.edn] [-p 8080]
  joker notebook run file.edn
  joker notebook status file.edn
  joker notebook deps file.edn
  joker notebook export file.edn [-o report.md]`)
}
