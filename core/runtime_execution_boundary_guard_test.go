package core

import (
	"os"
	"strings"
	"testing"
)

func TestExecutorFilesUseRuntimeExecutionAdapterForProgramState(t *testing.T) {
	for _, file := range []string{
		"boxed_exec.go",
		"typed_exec.go",
		"typed_exec_inline.go",
		"typed_exec_nanbox.go",
	} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for lineNo, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "prog.") {
				t.Fatalf("%s:%d reaches into IRProgram state instead of runtimeExec adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			if strings.Contains(line, "RuntimeExecutionAdapter{}") {
				t.Fatalf("%s:%d constructs ad-hoc runtime adapter instead of shared runtimeExec: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			if strings.Contains(line, "currentGRT()") || strings.Contains(line, ".Call(") || strings.Contains(line, "(Callable)") {
				t.Fatalf("%s:%d performs call/runtime dispatch instead of runtimeExec adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			if strings.Contains(line, "*Fn") || strings.Contains(line, "irGetFnProg") || strings.Contains(line, "wasmGetFn") || strings.Contains(line, ".env") {
				t.Fatalf("%s:%d reaches into Fn internals instead of runtimeExec adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			if strings.Contains(line, "ToSlice(") || strings.Contains(line, "(Seqable)") {
				t.Fatalf("%s:%d prepares call args instead of runtimeExec adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			if strings.Contains(line, "Seqable") || strings.Contains(line, "&ArrayVector") {
				t.Fatalf("%s:%d performs collection construction/access instead of runtimeExec adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
			if strings.Contains(line, "*StringCursor") || strings.Contains(line, ".Char()") || strings.Contains(line, ".Next()") || strings.Contains(line, ".Done()") {
				t.Fatalf("%s:%d performs cursor access instead of runtimeExec adapter: %s", file, lineNo+1, strings.TrimSpace(line))
			}
		}
	}
}
