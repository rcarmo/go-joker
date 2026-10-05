#!/usr/bin/env bash
# Network-enabled, post-tag check. Ordinary release-check stays offline.
set -euo pipefail

expected_tag=${1:?expected tag is required}
expected_revision=${2:?expected git revision is required}
module=github.com/rcarmo/go-joker/v42
[[ $expected_tag =~ ^v42\.[0-9]+\.[0-9]+$ ]] || { echo 'expected a v42 release tag' >&2; exit 1; }
[[ $expected_revision =~ ^[0-9a-f]{40}$ ]] || { echo 'expected a full git revision' >&2; exit 1; }

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
export PROFILE_ROOT=${PROFILE_ROOT:-$root/.cache/test-profiles/published-consumer}
tmp_root=${TMPDIR:-$(pwd)/.cache/tmp}
mkdir -p "$tmp_root"
work=$(mktemp -d "$tmp_root/joker-published-module.XXXXXX")
trap 'rm -rf "$work"' EXIT
cd "$work"
export GOWORK=off GOFLAGS=
# No direct fallback: verify the public module proxy as well as the tag.
export GOPROXY=${GOPROXY:-https://proxy.golang.org}
export GOPRIVATE= GONOPROXY=none GOSUMDB=sum.golang.org

go mod init example.com/joker-published-consumer
# New immutable tags can take a few minutes to reach the proxy/checksum DB.
# Retry availability, never change the requested version or bypass verification.
for attempt in 1 2 3 4 5; do
  if go get "$module@$expected_tag"; then break; fi
  if [[ $attempt == 5 ]]; then echo 'public module resolution failed' >&2; exit 1; fi
  sleep $((attempt * 15))
done
resolved=$(go list -m -f '{{.Path}}@{{.Version}}' "$module")
[[ $resolved == "$module@$expected_tag" ]] || { echo "wrong module: $resolved" >&2; exit 1; }
metadata=$(go mod download -json "$module@$expected_tag")
grep -Fq '"Hash": "'"$expected_revision"'"' <<<"$metadata" || {
  echo 'module version did not resolve to the release commit' >&2; exit 1;
}

cat > consumer_test.go <<EOF
package consumer
import (
 "strings"
 "testing"
 core "$module/core"
 runtime "$module/core/runtime"
 coretypes "$module/core/types"
)
func TestPublishedModule(t *testing.T) {
 if runtime.VERSION != "$expected_tag" { t.Fatalf("unexpected version: %s", runtime.VERSION) }
 obj, err := core.TryRead(core.NewReader(strings.NewReader("(+ 20 22)"), "<consumer>"))
 if err != nil { t.Fatal(err) }
 expr, err := core.TryParse(obj, &core.ParseContext{GlobalEnv: core.GLOBAL_ENV})
 if err != nil { t.Fatal(err) }
 result, err := core.TryEval(expr)
 if err != nil { t.Fatal(err) }
 n, ok := result.(coretypes.Int)
 if !ok || n.I != 42 { t.Fatalf("unexpected result: %v", result) }
}
EOF
go mod tidy
"$root/scripts/test-profile.sh" ./... -- -count=1 -v
# Exercise the documented install command, independent of the consumer's go.mod.
GOBIN="$work/bin" go install "$module/cmd/joker@$expected_tag"
"$work/bin/joker" --version 2>&1 | grep -Fx "$expected_tag"
echo "verified public module $resolved at $expected_revision"
