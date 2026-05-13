#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

go generate ./...
go vet ./...
go build -o joker ./cmd/joker
