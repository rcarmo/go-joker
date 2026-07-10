//go:build !plan9

package os

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"
)

func TestProcessBoundaryHelper(t *testing.T) {
	if os.Getenv("GO_JOKER_PROCESS_HELPER") != "1" {
		return
	}
	_, _ = os.Stdout.WriteString("stdout-boundary")
	_, _ = os.Stderr.WriteString("stderr-boundary")
	os.Exit(23)
}

func processTestOptions(args ...string) coretypes.Map {
	opts := corecollections.EmptyArrayMap()
	values := make([]coretypes.Object, len(args))
	for i, arg := range args {
		values[i] = coretypes.MakeString(arg)
	}
	return opts.Assoc(
		coretypes.MakeKeyword(STRINGS.Intern, "args"),
		corecollections.NewVectorFrom(values...),
	).(coretypes.Map)
}

func mapString(t *testing.T, m coretypes.Map, key string) string {
	t.Helper()
	ok, value := m.Get(coretypes.MakeKeyword(STRINGS.Intern, key))
	if !ok {
		t.Fatalf("result missing :%s", key)
	}
	return value.ToString(false)
}

func TestExecuteCapturesChildFailureBoundaries(t *testing.T) {
	t.Setenv("GO_JOKER_PROCESS_HELPER", "1")
	result := execute(os.Args[0], processTestOptions("-test.run=^TestProcessBoundaryHelper$")).(coretypes.Map)
	if got := mapString(t, result, "exit"); got != "23" {
		t.Fatalf("child exit = %s, want 23", got)
	}
	if got := mapString(t, result, "success"); got != "false" {
		t.Fatalf("child success = %s, want false", got)
	}
	if got := mapString(t, result, "out"); got != "stdout-boundary" {
		t.Fatalf("child stdout = %q", got)
	}
	if got := mapString(t, result, "err"); got != "stderr-boundary" {
		t.Fatalf("child stderr = %q", got)
	}
}

func TestExecuteRejectsMissingExecutableAndWorkingDirectory(t *testing.T) {
	t.Run("missing executable", func(t *testing.T) {
		defer func() {
			if recovered := recover(); recovered == nil || !strings.Contains(fmt.Sprint(recovered), "missing-command") {
				t.Fatalf("missing executable panic = %v", recovered)
			}
		}()
		_ = execute(filepath.Join(t.TempDir(), "missing-command"), corecollections.EmptyArrayMap())
	})

	t.Run("missing working directory", func(t *testing.T) {
		opts := processTestOptions("-test.run=^TestProcessBoundaryHelper$")
		opts = opts.Assoc(coretypes.MakeKeyword(STRINGS.Intern, "dir"), coretypes.MakeString(filepath.Join(t.TempDir(), "missing"))).(coretypes.Map)
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("missing working directory did not panic")
			}
		}()
		_ = execute(os.Args[0], opts)
	})
}
