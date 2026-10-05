#!/usr/bin/env bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/scripts/project-env.sh" || exit 1
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

"$CLI_BIN" tests/run-tests.joke --format tests/formatter out output.clj
