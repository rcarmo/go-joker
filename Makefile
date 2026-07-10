SHELL := /bin/bash

GO ?= go
TMPDIR ?= $(CURDIR)/.cache/tmp
GOTMPDIR ?= $(CURDIR)/.cache/gotmp
export TMPDIR
export GOTMPDIR
TOOLBIN := $(shell $(GO) env GOPATH)/bin
STATICCHECK_BIN ?= $(TOOLBIN)/staticcheck
GOLANGCI_LINT_BIN ?= $(TOOLBIN)/golangci-lint
GOVULNCHECK_BIN ?= $(TOOLBIN)/govulncheck

BENCH_REGEX ?= BenchmarkCLBG(NBody|Mandelbrot|SpectralNorm|BinaryTrees|FannkuchRedux)
COMPARE_OUT ?= benchmarks/compare/out/latest
DOCS_JOKER_BIN ?= $(TMPDIR)/go-joker-docs
CLI_BIN ?= .cache/tmp/joker
DIST_DIR ?= dist
DIST_PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

TEST_PKGS ?= ./...
TEST_TIMEOUT ?= 20m
TEST_COUNT ?= 1
TEST_SHUFFLE ?= off

.PHONY: help cli dist clean-dist tools test test-repro test-short test-core test-std vet staticcheck-sa lint vuln race bench-sanity compare-bench compare-clean coverage coverage-summary docs docs-command-check notebook-check notebook-browser-smoke notebook-screenshot examples-check docs-paths-check release-hygiene-check release-check pretag-check docs-check generated-check generated-bootstrap-check import-identity-check non-goals-check layout-check native-int-check error-handling-check benchmark-docs-check refactor-internals-check core-contract-check runtime-contract-check std-contract-check parity jank-subset bb-compat audit-fast audit

help:
	@echo "Available targets:"
	@echo ""
	@echo "Command-producing targets (build usable binaries):"
	@echo "  make cli            # Build the local joker CLI => $(CLI_BIN)"
	@echo "  make dist           # Build release CLIs => $(DIST_DIR)/joker-<os>-<arch>[.exe]"
	@echo "  make clean-dist     # Remove $(DIST_DIR)/ release binaries"
	@echo ""
	@echo "Targets that consume the local CLI artifact ($(CLI_BIN)):"
	@echo "  make parity         # Build $(CLI_BIN), then run Clojure parity tests"
	@echo "  make jank-subset    # Build $(CLI_BIN), then run imported jank subset"
	@echo ""
	@echo "Targets that build a temporary CLI internally for checks/docs:"
	@echo "  make docs           # Build temp CLI => $(DOCS_JOKER_BIN), then generate HTML docs"
	@echo "  make docs-command-check # Build temp CLI => $(DOCS_JOKER_BIN), then smoke-test doc queries"
	@echo "  make notebook-check # Build temp CLI => $(DOCS_JOKER_BIN), then verify notebook CLI/schema"
	@echo "  make notebook-browser-smoke # Build temp CLI => $(DOCS_JOKER_BIN), then run Playwright smoke test"
	@echo "  make notebook-screenshot # Build temp CLI => $(DOCS_JOKER_BIN), then capture notebook screenshot"
	@echo ""
	@echo "Test and audit targets:"
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
	@echo "  make docs-check     # Generate docs + verify new namespace/feature coverage"
	@echo "  make examples-check # Run runnable examples smoke checks"
	@echo "  make docs-paths-check # Guard stale moved-example paths in docs/examples"
	@echo "  make release-hygiene-check # Verify VERSION/README/release-note consistency"
	@echo "  make release-check  # Canonical local and CI release gate"
	@echo "  make pretag-check   # release-check plus optional browser smoke"
	@echo "  make generated-check # Verify generated-file boundary guardrails"
	@echo "  make generated-bootstrap-check # Verify generated bootstrap manifest equivalence"
	@echo "  make import-identity-check # Verify internal imports use github.com/rcarmo/go-joker"
	@echo "  make non-goals-check # Verify explicit non-goals remain documented"
	@echo "  make layout-check    # Verify top-level refactor layout invariants"
	@echo "  make native-int-check # Verify 32-bit/native-int audit TODOs are closed"
	@echo "  make error-handling-check # Verify close/process/raw-error audit guardrails"
	@echo "  make benchmark-docs-check # Verify benchmark docs/code stay in sync"
	@echo "  make refactor-internals-check # Run tests for extracted core helper subpackages"
	@echo "  make core-contract-check # Run object/protocol contract tests that gate future core splits"
	@echo "  make runtime-contract-check # Run IR/runtime execution-envelope contract tests"
	@echo "  make std-contract-check # Run focused std native-boundary contract tests"
	@echo "  make bb-compat      # Run portable Babashka compatibility fixture suite"
	@echo "  make compare-bench  # Run cross-runtime + let-go-suite comparison sub-project"
	@echo "  make compare-clean  # Remove generated comparison outputs"
	@echo "  make audit-fast     # test + vet + staticcheck + lint + vuln"
	@echo "  make audit          # full audit-fast + race + bench-sanity"

cli:
	@mkdir -p "$(TMPDIR)" "$(GOTMPDIR)" $$(dirname "$(CLI_BIN)")
	$(GO) build -o $(CLI_BIN) ./cmd/joker

clean-dist:
	rm -rf $(DIST_DIR)

dist: clean-dist
	@mkdir -p "$(TMPDIR)" "$(GOTMPDIR)" $(DIST_DIR)
	@set -euo pipefail; \
	for platform in $(DIST_PLATFORMS); do \
		goos=$${platform%/*}; goarch=$${platform#*/}; ext=""; \
		if [ "$$goos" = "windows" ]; then ext=".exe"; fi; \
		out="$(DIST_DIR)/joker-$$goos-$$goarch$$ext"; \
		echo "building $$out"; \
		GOOS=$$goos GOARCH=$$goarch CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o "$$out" ./cmd/joker; \
	done

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
	$(DOCS_JOKER_BIN) doc joker.core/first | grep -q '# \[`joker.core/first`\]'
	$(DOCS_JOKER_BIN) doc --format json joker.core/first | grep -q '"qualified": "joker.core/first"'
	$(DOCS_JOKER_BIN) doc joker.imaging/pixel | grep -q '# \[`joker.imaging/pixel`\]'

notebook-check:
	$(GO) test ./internal/notebook ./cmd/joker -run 'Test.*Notebook|TestEncodeLoad|TestFixtureLoad|TestRunCaptures|TestExportMarkdown|TestDownstream|TestBuildStatus|TestBuildDependencyGraph|TestDependencyCycles|TestUsageMentionsNotebookCommands' -count=$(TEST_COUNT)
	$(GO) build -o $(DOCS_JOKER_BIN) ./cmd/joker
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook new file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook demo'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook run file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q -- '--no-save'
	$(DOCS_JOKER_BIN) notebook --help | grep -q -- '--summary'
	$(DOCS_JOKER_BIN) notebook --help | grep -q -- '--fail-on-error'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook validate file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook status file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook deps file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q 'notebook snapshots file.edn'
	$(DOCS_JOKER_BIN) notebook --help | grep -q -- '--token secret'
	$(DOCS_JOKER_BIN) notebook --help | grep -q -- '--readonly'
	$(DOCS_JOKER_BIN) notebook validate tests/notebooks/basic.edn
	$(DOCS_JOKER_BIN) notebook validate tests/notebooks/rich_outputs.edn
	$(DOCS_JOKER_BIN) notebook validate tests/notebooks/dependencies.edn
	$(DOCS_JOKER_BIN) notebook validate examples/notebooks/rich-demo.edn
	$(DOCS_JOKER_BIN) notebook validate examples/notebooks/complex-demo.edn
	$(DOCS_JOKER_BIN) notebook run examples/notebooks/rich-demo.edn --no-save --summary --fail-on-error | grep -q '"success":true'
	$(DOCS_JOKER_BIN) notebook run examples/notebooks/complex-demo.edn --no-save --summary --fail-on-error | grep -q '"success":true'

notebook-browser-smoke:
	$(GO) build -o $(DOCS_JOKER_BIN) ./cmd/joker
	PLAYWRIGHT_BROWSERS_PATH=$(shell pwd)/.cache/ms-playwright JOKER_BIN=$(DOCS_JOKER_BIN) bun run scripts/notebook_smoke.ts

notebook-screenshot:
	$(GO) build -o $(DOCS_JOKER_BIN) ./cmd/joker
	PLAYWRIGHT_BROWSERS_PATH=$(shell pwd)/.cache/ms-playwright JOKER_BIN=$(DOCS_JOKER_BIN) bun run scripts/notebook_screenshot.ts

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

examples-check:
	tests/examples_smoke.sh .

docs-paths-check:
	tests/docs_paths_guard.sh .

release-hygiene-check:
	tests/release_hygiene_guard.sh .

release-check: release-hygiene-check
	git diff --check
	$(GO) vet ./...
	$(GO) test ./... -timeout 10m -count=1
	$(MAKE) docs-check

pretag-check: release-check
	@if [ "$${PRETAG_BROWSER_SMOKE:-0}" = "1" ]; then \
		$(MAKE) notebook-browser-smoke; \
	else \
		echo "skipping browser smoke; run PRETAG_BROWSER_SMOKE=1 make pretag-check to include it"; \
	fi

docs-check: docs docs-command-check notebook-check examples-check docs-paths-check release-hygiene-check generated-check generated-bootstrap-check import-identity-check non-goals-check layout-check native-int-check error-handling-check benchmark-docs-check refactor-internals-check core-contract-check runtime-contract-check std-contract-check
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
	test -f docs/API_STABILITY.md
	test -f docs/RELEASE_CHECKLIST.md
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

parity: cli
	$(GO) run tests/clojure_parity.go -joker $(abspath $(CLI_BIN)) -out docs/DIVERGENCE_MATRIX.md

jank-subset: cli
	JOKER_BIN=$(abspath $(CLI_BIN)) tests/run_jank_subset.sh

audit-fast: tools test-repro vet staticcheck-sa lint vuln

audit: audit-fast race bench-sanity
