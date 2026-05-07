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

.PHONY: help tools test vet staticcheck-sa lint vuln race bench-sanity audit-fast audit

help:
	@echo "Available targets:"
	@echo "  make tools          # Install/update audit tools (staticcheck, golangci-lint, govulncheck)"
	@echo "  make test           # Run full test suite"
	@echo "  make vet            # Run go vet"
	@echo "  make staticcheck-sa # Run staticcheck SA checks"
	@echo "  make lint           # Run focused golangci-lint profile"
	@echo "  make vuln           # Run govulncheck"
	@echo "  make race           # Run race tests on critical packages"
	@echo "  make bench-sanity   # Run CLBG benchmark sanity subset"
	@echo "  make audit-fast     # test + vet + staticcheck + lint + vuln"
	@echo "  make audit          # full audit-fast + race + bench-sanity"

tools:
	@echo "Installing/updating audit tooling..."
	@$(GO) install honnef.co/go/tools/cmd/staticcheck@latest
	@$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@$(GO) install golang.org/x/vuln/cmd/govulncheck@latest

test:
	$(GO) test ./...

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

audit-fast: tools test vet staticcheck-sa lint vuln

audit: audit-fast race bench-sanity
