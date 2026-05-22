SHELL := /bin/bash

GO ?= go
TMPDIR ?= /workspace/tmp
GOTMPDIR ?= /workspace/tmp
export TMPDIR
export GOTMPDIR
TOOLBIN := $(shell $(GO) env GOPATH)/bin
STATICCHECK_BIN ?= $(TOOLBIN)/staticcheck
GOLANGCI_LINT_BIN ?= $(TOOLBIN)/golangci-lint
GOVULNCHECK_BIN ?= $(TOOLBIN)/govulncheck

BENCH_REGEX ?= BenchmarkCLBG(NBody|Mandelbrot|SpectralNorm|BinaryTrees|FannkuchRedux)
COMPARE_OUT ?= benchmarks/compare/out/latest
DOCS_JOKER_BIN ?= /workspace/tmp/go-joker-docs

TEST_PKGS ?= ./...
TEST_TIMEOUT ?= 20m
TEST_COUNT ?= 1
TEST_SHUFFLE ?= off

.PHONY: help tools test test-repro test-short test-core test-std vet staticcheck-sa lint vuln race bench-sanity compare-bench compare-clean coverage coverage-summary docs docs-command-check notebook-check docs-check generated-check generated-bootstrap-check import-identity-check non-goals-check layout-check native-int-check error-handling-check refactor-internals-check core-contract-check runtime-contract-check std-contract-check parity jank-subset audit-fast audit

help:
	@echo "Available targets:"
	@echo "  make tools          # Install/update audit tools (staticcheck, golangci-lint, govulncheck)"
	@echo "  make test           # Run full test suite (cached)"
	@echo "  make test-repro     # Reproducible tests: no shuffle, no cache"
	@echo "  make test-short     # Reproducible short test run"
	@echo "  make test-core      # Reproducible core test run"
	@echo "  make test-std       # Reproducible std/* test run"
	@echo "  make vet            # Run go vet"
	@echo "  make staticcheck-sa # Run staticcheck SA checks"
	@echo "  make lint           # Run focused golangci-lint profile"
	@echo "  make vuln           # Run govulncheck"
	@echo "  make race           # Run race tests on critical packages"
	@echo "  make bench-sanity   # Run CLBG benchmark sanity subset"
	@echo "  make coverage       # Run package coverage and generated-file-aware summary"
	@echo "  make docs           # Generate HTML docs from runtime namespaces"
	@echo "  make docs-check     # Generate docs + verify new namespace/feature coverage"
	@echo "  make docs-command-check # Verify joker doc Markdown/JSON lookup smoke tests"
	@echo "  make notebook-check # Verify joker notebook schema/CLI smoke tests"
	@echo "  make generated-check # Verify generated-file boundary guardrails"
	@echo "  make generated-bootstrap-check # Verify generated bootstrap manifest equivalence"
	@echo "  make import-identity-check # Verify internal imports use github.com/rcarmo/go-joker"
	@echo "  make non-goals-check # Verify explicit non-goals remain documented"
	@echo "  make layout-check    # Verify top-level refactor layout invariants"
	@echo "  make native-int-check # Verify 32-bit/native-int audit TODOs are closed"
	@echo "  make error-handling-check # Verify close/process/raw-error audit guardrails"
	@echo "  make refactor-internals-check # Run tests for extracted core helper subpackages"
	@echo "  make core-contract-check # Run object/protocol contract tests that gate future core splits"
	@echo "  make runtime-contract-check # Run IR/runtime execution-envelope contract tests"
	@echo "  make std-contract-check # Run focused std native-boundary contract tests"
	@echo "  make parity         # Run Clojure parity tests (271 core form tests)"
	@echo "  make jank-subset    # Run imported jank-lang/clojure-test-suite subset"
	@echo "  make bb-compat      # Run portable Babashka compatibility fixture suite"
	@echo "  make compare-bench  # Run cross-runtime + let-go-suite comparison sub-project"
	@echo "  make compare-clean  # Remove generated comparison outputs"
	@echo "  make audit-fast     # test + vet + staticcheck + lint + vuln"
	@echo "  make audit          # full audit-fast + race + bench-sanity"

tools:
	@echo "Installing/updating audit tooling..."
	@$(GO) install honnef.co/go/tools/cmd/staticcheck@latest
	@$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@$(GO) install golang.org/x/vuln/cmd/govulncheck@latest

test:
	$(GO) test $(TEST_PKGS)

test-repro:
	$(GO) test -count=$(TEST_COUNT) -shuffle=$(TEST_SHUFFLE) -timeout=$(TEST_TIMEOUT) $(TEST_PKGS)

test-short:
	$(GO) test -short -count=$(TEST_COUNT) -shuffle=$(TEST_SHUFFLE) -timeout=$(TEST_TIMEOUT) $(TEST_PKGS)

test-core:
	$(GO) test -count=$(TEST_COUNT) -shuffle=$(TEST_SHUFFLE) -timeout=$(TEST_TIMEOUT) ./core

test-std:
	$(GO) test -count=$(TEST_COUNT) -shuffle=$(TEST_SHUFFLE) -timeout=$(TEST_TIMEOUT) ./std/...

vet:
	$(GO) vet ./...

staticcheck-sa: tools
	$(STATICCHECK_BIN) -checks=SA* ./...

lint: tools
	$(GOLANGCI_LINT_BIN) run --timeout=10m --exclude-files '(^|/)a_.*\.go$$' --disable-all -E govet -E staticcheck

vuln: tools
	$(GOVULNCHECK_BIN) ./...

race:
	$(GO) test -race ./core ./std/runtime ./std/http ./std/pdf

bench-sanity:
	$(GO) test ./benchmarks/core -run '^$$' -bench '$(BENCH_REGEX)' -benchmem -benchtime=1x -count=3

compare-bench:
	bash benchmarks/compare/collect.sh $(COMPARE_OUT)

compare-clean:
	rm -rf benchmarks/compare/out/latest

coverage:
	$(GO) test ./core ./std/... -coverprofile=$(TMPDIR)/go-joker.cover -covermode=atomic -timeout $(TEST_TIMEOUT) -count=$(TEST_COUNT)
	tests/coverage_summary.sh $(TMPDIR)/go-joker.cover $(TMPDIR)/go-joker.cover.func

coverage-summary:
	tests/coverage_summary.sh $(TMPDIR)/go-joker.cover $(TMPDIR)/go-joker.cover.func

docs:
	$(GO) build -o $(DOCS_JOKER_BIN) ./cmd/joker
	cd docs && $(DOCS_JOKER_BIN) generate-docs.joke > docs-generation.log && cat docs-generation.log && ! grep -q WARNING docs-generation.log && rm docs-generation.log

docs-command-check:
	$(GO) test ./cmd/joker -run 'TestRenderDoc|TestQueryDocs' -count=$(TEST_COUNT)
	$(GO) build -o $(DOCS_JOKER_BIN) ./cmd/joker
	$(DOCS_JOKER_BIN) doc joker.core/first | grep -q '# `joker.core/first`'
	$(DOCS_JOKER_BIN) doc --format json joker.core/first | grep -q '"qualified": "joker.core/first"'

notebook-check:
	$(GO) test ./internal/notebook ./cmd/joker -run 'Test.*Notebook|TestEncodeLoad|TestFixtureLoad|TestRunCaptures|TestExportMarkdown|TestDownstream|TestBuildStatus|TestBuildDependencyGraph|TestDependencyCycles|TestUsageMentionsNotebookCommands' -count=$(TEST_COUNT)
	$(GO) build -o $(DOCS_JOKER_BIN) ./cmd/joker
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook new file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook demo'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook run file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook validate file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook status file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook deps file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook snapshots file.edn'

bb-compat:
	$(GO) test ./tests -run Babashka -count=$(TEST_COUNT) -timeout=120s

generated-check:
	tests/generated_guard.sh .

generated-bootstrap-check:
	$(GO) test ./core ./core/generated -run 'TestGeneratedCoreSourceManifestRows|TestGeneratedCoreNamespacesHelper|TestGeneratedCoreNamespacesDriveCoreNamespaceVar' -count=$(TEST_COUNT)

import-identity-check:
	tests/import_identity_guard.sh .

non-goals-check:
	tests/non_goals_guard.sh .

layout-check:
	tests/layout_guard.sh .

native-int-check:
	tests/native_int_guard.sh .

error-handling-check:
	tests/error_handling_guard.sh .

benchmark-docs-check:
	$(GO) run tools/benchmarks/validate_readme_table.go
	cd benchmarks && python3 -m unittest run_benchmarks_test.py

refactor-internals-check:
	$(GO) test ./core/ir ./core/wasm ./core/trace ./core/generated ./core/hashutil ./core/types ./core/types/collections ./core/types/string ./core/types/numerical ./core/osutil ./core/bufferpool ./core/reader -count=$(TEST_COUNT)

core-contract-check:
	$(GO) test ./core -run 'TestCountedIndexedVectorContract|TestAssociativeMapContract|TestSetContract|TestSortedCollectionContract|TestTransientContract|TestSeqContract|TestInfoAndMetaContract|TestPVObjectSemantics|TestBigIntInt|TestRatioOrInt|TestReadIntegerUsesNativeIntRange|TestFileInfoMapPromotesLargeSize|TestReaderConstructionContract' -count=$(TEST_COUNT) -timeout=120s

runtime-contract-check:
	$(GO) test ./core -run 'TestIRExecutionMetadata|TestEscapeAnalysis|TestIRMakeFn|TestIRFunctionCache|TestRuntimeExecutionAdapter|TestExecutorFilesUseRuntimeExecutionAdapter|TestIRCompileFailure|TestNativeHelperEligibility|TestChannelCloseIsIdempotentUnderConcurrency|TestWasmRawInt|TestWasmExecRawIntegerResultUsesNativeRange' -count=$(TEST_COUNT) -timeout=120s
	$(GO) test ./core/runtime -run 'TestAgent|TestAtom|TestChannel|TestObjectChannel|TestFuture|TestObjectFuture|TestPromise|TestObjectPromise|TestCheckedMillisecondDuration|TestRunParallel|TestFeatureFlag|TestIRInlineMode|TestIRTypedMapMode|TestGoID' -count=$(TEST_COUNT) -timeout=120s

std-contract-check:
	$(GO) test ./std/... -count=$(TEST_COUNT) -timeout=120s

docs-check: docs docs-command-check notebook-check generated-check generated-bootstrap-check import-identity-check non-goals-check layout-check native-int-check error-handling-check benchmark-docs-check refactor-internals-check core-contract-check runtime-contract-check std-contract-check
	test -f docs/refactor/README.md
	test -f docs/refactor/code-structure.md
	test -f docs/refactor/module-structure-audit.md
	test -f docs/refactor/module-structure-followup.md
	test -f docs/refactor/ir-boundary.md
	test -f docs/refactor/ir-program-split.md
	test -f docs/refactor/generated-boundary.md
	test -f docs/refactor/generated-bootstrap-contract.md
	test -f docs/refactor/core-split.md
	test -f docs/refactor/object-protocol-contracts.md
	test -f docs/refactor/runtime-execution-contract.md
	test -f docs/refactor/reader-construction-contract.md
	test -f docs/BABASHKA_SHIM_ASSESSMENT.md
	test -f docs/PORTABILITY_SHIM_ASSESSMENT.md
	test -f docs/BENCHMARK_CI.md
	test -f docs/joker.imaging.html
	test -f docs/joker.jit.html
	test -f docs/joker.edn.html
	test -f docs/edn.html
	test -f docs/pods.html
	test -f docs/babashka.pods.html
	test -f docs/joker.transit.html
	test -f docs/joker.log.html
	test -f docs/joker.pdf.html
	test -f docs/joker.random.html
	test -f docs/joker.svg.html
	grep -q 'id="joker.log"' docs/index.html
	grep -q 'id="joker.random"' docs/index.html
	grep -q 'id="joker.imaging"' docs/index.html
	grep -q 'id="joker.edn"' docs/index.html
	grep -q 'id="edn"' docs/index.html
	grep -q 'id="pods"' docs/index.html
	grep -q 'id="babashka.pods"' docs/index.html
	grep -q 'id="joker.transit"' docs/index.html
	grep -q 'id="joker.pdf"' docs/index.html
	grep -q 'id="joker.svg"' docs/index.html
	grep -q 'WebSocket upgrade extension' docs/joker.http.html
	grep -q 'SSE/chunked streaming extension' docs/joker.http.html
	grep -q 'id="alts!"' docs/joker.core.html
	grep -q 'id="timeout"' docs/joker.core.html
	grep -q 'id="future"' docs/joker.core.html
	grep -q 'id="promise"' docs/joker.core.html
	grep -q 'id="agent"' docs/joker.core.html
	grep -q 'id="pmap"' docs/joker.core.html
	grep -q 'id="pcalls"' docs/joker.core.html

parity:
	$(GO) run tests/clojure_parity.go -joker $(shell pwd)/joker -out docs/DIVERGENCE_MATRIX.md

jank-subset:
	JOKER_BIN=$(shell pwd)/joker tests/run_jank_subset.sh

audit-fast: tools test vet staticcheck-sa lint vuln

audit: audit-fast race bench-sanity
