#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

./joker tests/run-tests.joke --format tests/formatter out output.clj
