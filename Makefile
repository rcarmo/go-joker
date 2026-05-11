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

.PHONY: help tools test test-repro test-short test-core test-std vet staticcheck-sa lint vuln race bench-sanity compare-bench compare-clean coverage coverage-summary docs docs-check generated-check import-identity-check non-goals-check refactor-internals-check parity jank-subset audit-fast audit

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
	@echo "  make generated-check # Verify generated-file boundary guardrails"
	@echo "  make import-identity-check # Verify internal imports use github.com/rcarmo/go-joker"
	@echo "  make non-goals-check # Verify explicit non-goals remain documented"
	@echo "  make refactor-internals-check # Run tests for extracted core/internal packages"
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
	$(GO) test ./core -run '^$$' -bench '$(BENCH_REGEX)' -benchmem -benchtime=1x -count=3

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

bb-compat:
	$(GO) test ./tests -run Babashka -count=$(TEST_COUNT) -timeout=120s

generated-check:
	tests/generated_guard.sh .

import-identity-check:
	tests/import_identity_guard.sh .

non-goals-check:
	tests/non_goals_guard.sh .

refactor-internals-check:
	$(GO) test ./core/internal/... -count=$(TEST_COUNT)

docs-check: docs generated-check import-identity-check non-goals-check refactor-internals-check
	test -f docs/ARCHITECTURE_REFACTOR_PLAN.md
	test -f docs/IR_BOUNDARY_AUDIT.md
	test -f docs/GENERATED_BOUNDARY_AUDIT.md
	test -f docs/CORE_SPLIT_AUDIT.md
	test -f docs/BABASHKA_SHIM_ASSESSMENT.md
	test -f docs/PORTABILITY_SHIM_ASSESSMENT.md
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
