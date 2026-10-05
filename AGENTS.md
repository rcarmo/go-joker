<!-- RUI-PROFILE-LIFECYCLE-20261005 -->
## Current profiling and cleanup rule — supersedes older text below

Rui's explicit rule: **profile and tune during pre-release tests; remove profiling data immediately after analysis/use.** Ordinary development tests do not require profiling on every run. Targeted diagnostic profiling is optional when useful and follows the same disposal rule.

- During pre-release verification, capture CPU and heap/allocation behaviour, analyse hotspots and tune avoidable allocations/repeated work. Compare equivalent workloads without weakening correctness/security/numerical contracts. For Go, inspect CPU, alloc_space and alloc_objects; coverage alone is not profiling.
- Keep captures and matching artifacts only while the current analysis needs them. Once used, immediately delete raw profiles, traces, matching test binaries, temporary fixtures and disposable logs, including failed/probe artifacts after diagnosis. Retain only concise conclusions and important measurements/limitations. Do not keep indefinite raw archives or copy them into exports/reports/evidence to evade cleanup.
- Remove all completed disposable cache/build/test/run data promptly. Never delete files still in use: finish or safely pause the owning job and clean at a safe boundary. Preserve source, installed toolchains, durable datasets/checkpoints and intentional release assets. Minimise disk usage; no random exports or redundant snapshots.
- This rule overrides **every conflicting older paragraph in this file and linked local guidance**, including “profile every test”, “unprofiled tests prohibited”, “retain all raw evidence”, “never delete profiles” and cleanup exclusions based only on an evidence/profiles directory name. Update helper/CI cleanup behaviour accordingly; do not weaken pre-release analysis.
- Existing no-agent-contact and execution-pause rules remain unchanged. This policy grants no unsolicited coordination or automatic job restart.
<!-- /RUI-PROFILE-LIFECYCLE-20261005 -->

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

## Working rules

- Read relevant code and contracts before editing; keep fixes small and follow YAGNI.
- Inspect Git status first and preserve unrelated work. Never use `git rebase`; integrate with `git merge` or `git pull --no-rebase`.
- Commit semantic fixes separately from performance changes. Do not push tags, publish releases, or change portfolio claims without operator approval.
- Keep the working plan current. Report concrete results, failed attempts and remaining blockers; do not mark unverified work complete.

## Project Structure

```
core/           # Core interpreter (parser, evaluator, data types)
core/data/      # Core Joker libraries (.joke files: core.joke, test.joke, repl.joke)
core/gen/codegen/  # Code generation for building Joker
core/gen/gengo/    # Go code generation helpers
std/            # Standard library wrappers (.joke definitions + Go implementations)
std/*/          # Go implementations for each std namespace
tests/          # Test suites (eval, linter, formatter, flags)
docs/           # Documentation generation
```

## Project-owned caches and scratch (mandatory 2026-10-05)

- Canonical host root: `/workspace/tmp/go-joker/`. Reproducible caches go in `cache/<tool>/`, binaries/crossbuilds in `build/`, isolated disposable work in `runs/<purpose>/<run-id>/`. Use portable project-named fallback roots only when the workspace host is absent; never create bare/ad-hoc temporary or home cache paths.
- Source `scripts/project-env.sh` before direct commands. Make recipes, test helpers and Bun entrypoints load the same checked configuration. `TMPDIR`/`TMP`/`TEMP`, `GOTMPDIR`, `GOCACHE`, `GOMODCACHE`, `GOPATH`/`GOBIN`, Bun/npm/Python cache variables and Playwright downloads are routed to this root. Do not override them outside it.
- `PROJECT_TMP_BASE` selects `<absolute-base>/go-joker`; compatible `PROJECT_TMP_ROOT` selects an absolute project-named root. If both are supplied they must agree; empty, relative, unsafe or unusable overrides fail, never silently fall back.
- Snapshot inherited `TMPDIR` as `PROJECT_ORIGINAL_TMPDIR` before resolving. Without overrides, **CI selects usable `RUNNER_TEMP`, then original TMPDIR, then platform temp**, even if `/workspace/tmp` exists. Local use selects usable `/workspace/tmp`, then platform temp. Always append `go-joker`, resolve once and export the root to children so redirected TMPDIR cannot nest roots. CI vendors the resolver and does not depend on the host Makefile.
- Stable layout: `cache/<tool>/`, `build/`, `tests/`, `logs/`, `runs/<purpose>/<run-id>/`. Test isolation/ownership checks remain mandatory; raw captures are disposable after analysis; durable assets stay protected.
- Raw CPU/heap captures, matching test binaries/traces/run logs and completed scratch are disposable immediately after analysis/use. Keep concise conclusions only; preserve durable source, fixtures, screenshots, release assets and active-job files. Profile runs use the canonical `runs/profiles` root.
- `make clean-cache CLEAN_CONFIRM=go-joker` removes only canonical rebuildable cache/build directories after jobs stop. Dispose completed profile runs immediately after review; never remove active runs, source, other projects, concise conclusions or durable release assets. Test-owned isolated directories stay beneath `runs`; preserve ownership/symlink guards.
- Dispose completed rebuildable caches/scratch after jobs stop; never touch active-job files or another owner's project. Python profiling is not part of this policy.

## Build Commands

Use the versions declared in `go.mod` (currently Go 1.25 minimum, Go 1.26.5 toolchain). Record the actual toolchain used for measurements.

```bash
source scripts/project-env.sh
make cli                         # /workspace/tmp/go-joker/build/joker
"$CLI_BIN" --version
go build -tags go_spew -o "$PROJECT_TMP_ROOT/build/joker-debug" ./cmd/joker
```

Normal builds use committed generated sources. Full bootstrap regeneration has known extracted-package/generic serialization limitations; do not run it as a routine build step or hand-edit generated output to work around failures. See the generated-file section below.

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
- `make` loads `scripts/project-env.sh` and exports the canonical project-owned temp/cache configuration.
- If tooling is missing, targets auto-bootstrap required binaries via `make tools`.

## Test Commands

```bash
# Run all tests
./tools/scripts/all-tests.sh

# Individual test suites
./tools/scripts/eval-tests.sh          # Core evaluation tests
./tools/scripts/linter-tests.sh        # Linter functionality tests
./tools/scripts/formatter-tests.sh     # Formatter tests
./tools/scripts/flag-tests.sh          # Command-line flag tests

# Run a single test file
/workspace/tmp/go-joker/build/joker tests/run-eval-tests.joke tests/eval/<test-name>.joke
# Example:
/workspace/tmp/go-joker/build/joker tests/run-eval-tests.joke tests/eval/core.joke

# Lint Go code for shadowed variables
./tools/scripts/shadow.sh

# Lint Clojure/Joker files
/workspace/tmp/go-joker/build/joker --lint <file.clj>

# Format Clojure/Joker files
/workspace/tmp/go-joker/build/joker --format <file.clj>
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

    . "github.com/rcarmo/go-joker/v42/core"
    _ "github.com/rcarmo/go-joker/v42/std/string"
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

Full regeneration commands (use only for generator/source work, on a clean reviewed baseline):
```bash
go generate ./...
(cd std; /workspace/tmp/go-joker/build/joker generate-std.joke)
```

These commands currently have known bootstrap and std binding failures. Preserve diagnostics, review generated diffs and restore invalid output rather than committing an unbuildable tree. Generated code must come from a generator, never manual edits. For the five arithmetic docstrings, the checked narrow synchroniser is supported:

```bash
bun tools/codegen/sync-arithmetic-docs.ts
bun tools/codegen/sync-arithmetic-docs.ts --check
make docs                         # regenerate static API HTML
make docs-verify                  # compare without modifying tracked docs
```

## Adding New Code

### Adding a Core Namespace

1. Create `core/data/<namespace>.joke`
2. Add to `CoreSourceFiles` array in `core/gen/codegen/main.go`
3. Add tests in `tests/eval/`
4. Run the focused tests and full regression gate below

### Adding a Std Namespace

1. Create `std/<name>.joke` with `:go` metadata
2. `mkdir -p std/<name>`
3. `(cd std; /workspace/tmp/go-joker/build/joker generate-std.joke)`
4. Write supporting Go code in `std/<name>/<name>_native.go`
5. Add the registration import in the appropriate `cmd/joker` initialization file
6. Add tests in `tests/eval/`
7. Rebuild and test

## Important Notes

- Follow the Go minimum/toolchain declared in `go.mod`
- Build artifacts (`a_*.go`) are committed to the repo due to circular dependencies
- Use `make cli` for normal builds; full regeneration is not a prerequisite
- Core namespaces are listed in `joker.core/*core-namespaces*`

## Correctness audit criteria

- Record commit, dirty-tree state, Go/runtime versions, OS/architecture, CPU features, build flags and host load before an audit. Read `docs/EXECUTION_TIER_AUDIT.md` and the runtime/IR contracts under `docs/refactor/` for prior findings and limitations.
- Compare the same program, inputs and initial state across applicable native integer, WASM, typed/inline/NaN-boxed IR, boxed IR and interpreter paths. Use supported controls or narrow test hooks and prove which executor ran. Silent fallback is not proof of an optimised tier.
- Compare values **and types**, exceptions, stdout where relevant, mutations and evaluation order. Reset state and control nondeterminism. Prioritise overflow/promotion, exact division, NaNs, closure capture, Var rebinding, recursion, collections/transients and fallback transitions.
- The tree-walker is a practical oracle, not an unquestionable one: verify disputed semantics against primitive contracts, tests and docs. Do not impose Clojure/JVM behaviour on intentional Joker differences.
- Minimise each discrepancy, retain deterministic seeds, and show a regression failing before and passing after the fix. Include unaffected tiers and representative scripts/notebooks.
- Never replay side-effectful execution to hide a failure. Do not disable tiers globally or loosen comparisons to pass tests. If pre-execution rejection is the correct unsupported-shape contract, test and document its eligibility and performance trade-off.
- Use bounded fuzz/property tests where useful. Report tested scope and residual risks; a bounded audit is not proof of universal tier equivalence.

## Profiling, allocation and performance criteria

- Pre-release Go checks must capture CPU/heap profiles, inspect cumulative CPU plus alloc_space/alloc_objects, and tune our measured bottlenecks. Ordinary development checks do not have to profile: use PROFILE_TESTS=1 when profiling is needed.
- Raw captures, matching binaries/traces/logs are kept only during analysis/use. Immediately run scripts/dispose-profiles.sh <run> "concise conclusions" after review; it records compact findings and deletes the raw run. CI performs compact diagnostic review and disposes captures; do not upload raw profiles.
- Profiles use the canonical project runs/profiles hierarchy; concise useful conclusions use .cache/profile-conclusions. Missing profiles after failed builds/crashes must be reported. Never use empty CPU samples as a performance result; longer representative workloads are required for CPU attribution. Do not optimize dependencies/tests merely to hide our costs or loosen acceptance thresholds.
- Profile representative workloads before choosing an optimisation. Collect CPU profiles and allocation profiles/counts; report time/op, bytes/op and allocations/op where applicable. Use `./benchmarks/core`, not the old `./core` benchmark location, and verify a requested benchmark actually ran.
- Check load before timing. Run baseline/candidate sequentially on the same runner with identical inputs, toolchain, flags and scheduler settings. Use repeated matched samples; keep instrumentation out of timings. Separate startup, parsing, compilation and execution costs.
- Follow `docs/BENCHMARK_CI.md`: prefer ten samples, use the pinned `go tool benchstat`, and run `tests/benchmark_regression_check.py`. Current policy requires at least six samples and identical benchmark sets; gates stable timing growth above 15%, stable bytes growth above 5% and at least eight bytes, and any median allocation increase. Investigate noise or regressions; do not weaken the policy to accept a candidate.
- Validate output before timing and retain raw samples, environment details and commands. Pair primitive microbenchmarks with a representative workload before making workload-level claims. Reject unjustified optimisations, even if a microbenchmark improves.
- Distinguish portable interpreted, native-helper, and WASM workloads. Never attribute a best-Joker/native Mandelbrot result specifically to WASM. Historical charts are snapshots, not controlled patch comparisons.
- Refresh charts only when requested, rerunning comparison runtimes rather than mixing new Joker timings with stale baselines. Archive previous data, label changed fixtures (notably corrected integer-quotient pidigits), and exclude incompatible baseline speedups. Keep current tables, generators and docs consistent.

## SIMD criteria

SIMD is an explicit optimisation target where available, with a correct scalar fallback on **every supported architecture**.

- Identify a measured bottleneck and check existing vectorised library/compiler paths before adding assembly or dependencies. Consider native SIMD and WASM SIMD separately.
- Dispatch only after supported CPU/OS feature checks; retain baseline builds (including `GOAMD64=v1`). No unsupported instructions may run during initialization or on fallback hosts. Use architecture/build constraints and portable implementations; AVX2/SSE or arm64 NEON availability is not permission to drop other targets.
- Differential-test SIMD against scalar on identical data. Cover zero/short inputs, vector-width boundaries and tails, unaligned slices, aliasing, page/buffer boundaries, overflow, NaNs and Unicode where relevant. No out-of-bounds reads, unsafe alignment assumptions, or semantic changes for speed.
- Provide a testable way to exercise scalar and each available SIMD path; verify actual selection. Do not claim SIMD coverage from fallback results or cross-compilation alone.
- Measure representative benefits with the same repeated timing/allocation policy. Retain scalar code as the portable reference; report architectures executed, only cross-built, unavailable or unsupported.

## Mandatory full regression validation

Focused tests are development aids, not a substitute for the full suite after changes. Required validation includes:

```bash
make test-repro                  # repository-wide reproducible Go tests
PRETAG_BROWSER_SMOKE=1 make pretag-check
make race                       # race suite; report any blocker explicitly
bun tools/codegen/sync-arithmetic-docs.ts --check
```

The pre-tag gate includes repository-wide tests and vet, Joker AI lint/fixtures, docs/generated/layout/contract guards, examples and notebooks. Browser smoke must be enabled for final validation; install package/browser dependencies first as documented in CI. Also run relevant eval/linter/formatter/flag fixtures when not covered by these targets or when their behaviour changes.

Run scalar/SIMD differential tests, bounded fuzzing and benchmark regression checks for affected paths. Cross-build supported release platforms and execute platform tests where available, including Linux/386 for numeric width changes. Cross-build success is not execution coverage. Record skipped, failed or unavailable tests and do not describe a partial run as full validation.

Seek independent adversarial review for audit/sign-off when an approved executable model or human reviewer is available. Inspect actual availability; never invent a model or bypass delegate policy. Report unavailable review honestly.

## C ABI and SDL sample

- `std/ffi` registers `joker.ffi`; native calls are enabled in normal Linux/macOS/Windows amd64/arm64 builds and release binaries with `CGO_ENABLED=0`. No feature tag is required. Unsupported targets retain unavailable errors. Do not introduce an unrequested opt-in gate.
- Run `make ffi-check` and the profiled `make sdl-fluid-screenshot` for ABI/sample changes. Keep SDL calls in `examples/graphics/sdl-fluid/fluid.joke`; simulation/image buffers belong to the example host, not the FFI namespace.
- Fixed signatures only: no guessed C varargs, callbacks, struct layouts or unsafe unload. Preserve opaque pointer provenance and distinct callable identity. Native buffers cannot be retained by C; embedded hosts own thread/authority policy.
- Screenshots are actual SDL readback and documentation; never use image hashes/byte equality as fluid acceptance. Test finite fields, physical/output bounds and rounding-calibrated numerical/image error tolerances.

## Front-end and release hygiene

- Front-end updates include `package.json`, `bun.lock`, embedded assets under `internal/notebook/assets/`, and browser smoke. Pin direct versions, verify vendored files against installed packages, and document major-version migrations separately (CodeMirror v6 is not a drop-in v5 patch).
- Keep runtime version, README and versioned release notes synchronized. Follow `docs/RELEASE_CHECKLIST.md` and `docs/RELEASE_SUPPLY_CHAIN.md` only after release approval.
- Before publication, require clean reviewed changes and passing gates. Push the intended branch/tag and verify their remote commit identities; never rebase or silently overwrite a published release tag.
- After publication, verify workflow success, the six binaries/six SBOMs/checksum manifest, downloaded checksums, embedded revision/version and provenance. Report remaining limitations; never turn benchmark results into broader portfolio claims without approval.
