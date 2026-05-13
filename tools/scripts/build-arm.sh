#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

GOOS=linux GOARCH=386 go generate ./...
GOOS=linux GOARCH=arm go build -o joker ./cmd/joker
