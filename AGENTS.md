# AGENTS.md - Joker Codebase Guide

This document provides guidance for AI coding agents working in the Joker codebase.
Joker is a small Clojure interpreter, linter, and formatter written in Go.

## Git Identity

All commits in this repository must use:

- Author: `Rui Carmo <rui.carmo@gmail.com>`
- Committer: `Rui Carmo <rui.carmo@gmail.com>`

Configure both local and global Git identity before committing:

```bash
git config user.name "Rui Carmo"
git config user.email "rui.carmo@gmail.com"
git config --global user.name "Rui Carmo"
git config --global user.email "rui.carmo@gmail.com"
```

## Project Structure

```
core/           # Core interpreter (parser, evaluator, data types)
core/data/      # Core Joker libraries (.joke files: core.joke, test.joke, repl.joke)
core/gen_code/  # Code generation for building Joker
core/gen_go/    # Go code generation helpers
std/            # Standard library wrappers (.joke definitions + Go implementations)
std/*/          # Go implementations for each std namespace
tests/          # Test suites (eval, linter, formatter, flags)
docs/           # Documentation generation
```

## Build Commands

```bash
# Full build (recommended) - cleans, generates, vets, builds, regenerates std
./run.sh --build-only

# Quick build - generates, vets, builds
./build.sh

# Manual build steps
go generate ./...        # Generate code from .joke files
go vet ./...             # Static analysis
go build ./cmd/joker    # Compile

# Build with debugging tools
go build -tags go_spew ./cmd/joker   # Enables joker.core/go-spew debugging function
```

## Preferred workflow: Makefile targets

When possible, prefer `make` targets over ad-hoc commands. They centralize flags and are reproducible across environments.

```bash
# Discover available targets
make help

# Reproducible full test run (recommended default for agents)
make test-repro

# Deterministic subsets
make test-core
make test-std
make test-short

# Full audit pipeline
make audit-fast   # test + vet + staticcheck + lint + vuln
make audit        # audit-fast + race + benchmark sanity
```

Useful overrides:

```bash
# Change package scope and timeout without editing the Makefile
make test-repro TEST_PKGS=./core TEST_TIMEOUT=30m
```

Notes:
- `make` exports `TMPDIR`/`GOTMPDIR` to avoid noexec `/tmp` issues in constrained environments.
- If tooling is missing, targets auto-bootstrap required binaries via `make tools`.

## Test Commands

```bash
# Run all tests
./all-tests.sh

# Individual test suites
./eval-tests.sh          # Core evaluation tests
./linter-tests.sh        # Linter functionality tests
./formatter-tests.sh     # Formatter tests
./flag-tests.sh          # Command-line flag tests

# Run a single test file
./joker tests/run-eval-tests.joke tests/eval/<test-name>.joke
# Example:
./joker tests/run-eval-tests.joke tests/eval/core.joke

# Lint Go code for shadowed variables
./shadow.sh

# Lint Clojure/Joker files
./joker --lint <file.clj>

# Format Clojure/Joker files
./joker --format <file.clj>
```

## Test File Organization

- **Eval tests**: `tests/eval/*.joke` - Unit tests using joker.test
- **Forked tests**: `tests/eval/*/` - Directories with `input.joke`, `stdout.txt`, `stderr.txt`, `rc.txt`
- **Linter tests**: `tests/linter/*/` - Directories with `input.clj` and expected `output.txt`
- **Formatter tests**: `tests/formatter/*/` - Input and expected output files

## Code Style Guidelines

### Imports

Organize imports in groups separated by blank lines:
1. Standard library (alphabetically sorted)
2. Third-party packages
3. Local project imports

```go
import (
    "fmt"
    "strings"

    "github.com/pkg/profile"

    . "github.com/rcarmo/go-joker/core"
    _ "github.com/rcarmo/go-joker/std/string"
)
```

Note: The core package often uses dot imports (`.`). Std packages use blank identifier imports (`_`) for side-effect registration.

### Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| Exported types | PascalCase | `Symbol`, `ReplContext` |
| Unexported types | camelCase | `pos`, `internalInfo` |
| Exported functions | PascalCase | `MakeSymbol`, `NewReplContext` |
| Unexported functions | camelCase | `processFile`, `escapeRune` |
| Variables | camelCase | `dataRead`, `posStack` |
| Constants (global) | SCREAMING_SNAKE_CASE | `EOF`, `VERSION` |
| Constants (enums) | PascalCase | `READ`, `PARSE`, `EVAL` |
| Proc functions | camelCase with context | `procMeta`, `procWithMeta` |

### Type Patterns

**Small, focused interfaces:**
```go
type Equality interface {
    Equals(interface{}) bool
}
```

**Struct embedding for composition:**
```go
type Symbol struct {
    InfoHolder   // Position info
    MetaHolder   // Metadata
    ns   *string
    name *string
    hash uint32
}
```

**Method receivers - pointer for mutable/large, value for immutable/small:**
```go
func (a *Atom) Deref() Object { ... }    // Pointer - mutable
func (c Char) ToString(escape bool) string { ... }  // Value - immutable
```

### Error Handling

**Pattern 1: Panic with custom error types (most common in core)**
```go
func CheckArity(args []Object, min int, max int) {
    if n := len(args); n < min || n > max {
        PanicArityMinMax(n, min, max)
    }
}
```

**Pattern 2: PanicOnErr helper**
```go
f, err := filepath.Abs(filename)
PanicOnErr(err)
```

**Pattern 3: EnsureObjectIs* assertions**
```go
str := EnsureObjectIsString(obj, "pattern")
num := EnsureArgIsNumber(args, 0)
```

**Pattern 4: Deferred recovery with type switching on ParseError, EvalError, Error**

### File Naming

| Pattern | Purpose |
|---------|---------|
| `a_*.go` | Generated files (do not edit manually) |
| `*_native.go` | Hand-written Go implementations for std |
| `*_slow_init.go` | Slow-startup initialization code |
| `*_fast_init.go` | Fast-startup initialization code |
| `*_plan9.go`, `*_windows.go` | Platform-specific code |

### Generated Files

Files prefixed with `a_` are auto-generated. Do not edit them directly:
- `core/a_*.go` - Generated from core/data/*.joke
- `std/*/a_*.go` - Generated from std/*.joke

To regenerate:
```bash
go generate ./...                    # Regenerate core
(cd std; ../joker generate-std.joke) # Regenerate std
```

## Adding New Code

### Adding a Core Namespace

1. Create `core/data/<namespace>.joke`
2. Add to `CoreSourceFiles` array in `core/gen_code/gen_code.go`
3. Add tests in `tests/eval/`
4. Run `./run.sh --build-only && ./all-tests.sh`

### Adding a Std Namespace

1. Create `std/<name>.joke` with `:go` metadata
2. `mkdir -p std/<name>`
3. `(cd std; ../joker generate-std.joke)`
4. Write supporting Go code in `std/<name>/<name>_native.go`
5. Add import to `main.go`
6. Add tests in `tests/eval/`
7. Rebuild and test

## Important Notes

- Joker requires Go 1.24.0+
- Build artifacts (`a_*.go`) are committed to the repo due to circular dependencies
- The `run.sh` script handles the full build cycle including code generation
- Use `--build-only` flag to skip running Joker after building
- Core namespaces are listed in `joker.core/*core-namespaces*`
