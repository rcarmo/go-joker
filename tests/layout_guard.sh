#!/usr/bin/env bash
set -euo pipefail

root=${1:-.}
cd "$root"

status=0
fail() { echo "layout guard: $*" >&2; status=1; }

[[ -f cmd/joker/main.go ]] || fail "missing CLI entrypoint cmd/joker/main.go"
for dir in trace ir wasm generated runtime reader types; do
  [[ -d "core/${dir}" ]] || fail "missing core/${dir}"
done
for dir in runtime reader ir wasm generated types; do
  [[ -f "core/${dir}/doc.go" ]] || fail "missing core/${dir}/doc.go"
done
[[ -f core/types/collections/doc.go ]] || fail "missing core/types/collections/doc.go"
[[ -f core/types/string/doc.go ]] || fail "missing core/types/string/doc.go"
[[ -f core/types/numerical/doc.go ]] || fail "missing core/types/numerical/doc.go"
[[ -f std/http/router/router.joke ]] || fail "missing std/http/router/router.joke"

if [[ -d lib/joker/http ]]; then
  fail "loose HTTP libraries under lib/joker/http are not allowed; place joker.http.* resources under std/http/"
fi

while IFS= read -r joke; do
  rel=${joke#std/}
  ns=${rel%%/*}
  if [[ ! -f "std/${ns}.joke" ]]; then
    fail "nested std resource ${joke} requires root namespace file std/${ns}.joke"
  fi
  if [[ ${rel#*/} != */* ]]; then
    fail "nested std resource ${joke} must live under std/${ns}/<subns>/..."
  fi
done < <(find std -mindepth 2 -type f -name '*.joke' | sort)

if find . -maxdepth 1 -type f -name '*.go' | grep -q .; then
  find . -maxdepth 1 -type f -name '*.go' -print >&2
  fail "root package Go files are not allowed; CLI belongs in cmd/joker and runtime code belongs in packages"
fi

allowed_root_core_files=(
  core/a_generated_bootstrap_payloads.go
  core/bootstrap_gen_code.go
  core/error_frame_runtime.go
  core/fn_ir_cache_runtime.go
  core/frequencies_fast_runtime.go
  core/int_range_runtime.go
  core/ir_call_dispatch_runtime.go
  core/reader_construction_runtime.go
  core/reducible_pipeline_runtime.go
  core/runtime_kernel.go
  core/runtime_kernel_contracts_test.go
  core/runtime_kernel_spew_enabled.go
  core/seq_ops_fast_runtime.go
  core/tagged_literals_runtime.go
  core/tail_call_runtime.go
  core/transducer_compat_runtime.go
  core/wasm_compile_runtime.go
  core/wasm_loop_wrapper_runtime.go
)
for required in "${allowed_root_core_files[@]}"; do
  [[ -f "$required" ]] || fail "missing expected coalesced root kernel file: $required"
done
while IFS= read -r file; do
  allowed=false
  for expected in "${allowed_root_core_files[@]}"; do
    if [[ "$file" == "$expected" ]]; then
      allowed=true
      break
    fi
  done
  if [[ "$allowed" != true ]]; then
    fail "unexpected root core file $file; coalesce into runtime_kernel.go or move to a real package"
  fi
done < <(find core -maxdepth 1 -type f -name '*.go' | sort)

while IFS= read -r file; do
  if grep -q '^package core$' "$file"; then
    fail "package core file in subpackage directory is not a real package move: $file"
  fi
done < <(find core -mindepth 2 -type f -name '*.go' ! -path 'core/gen/*' | sort)

if grep -R '^func Benchmark' core --include='*_test.go' >/dev/null; then
  grep -R '^func Benchmark' core --include='*_test.go' >&2
  fail "core package benchmark functions belong under benchmarks/core, not root core"
fi

moved_collection_files=(
  array_map.go
  array_vector.go
  chunked_seq.go
  hash_map.go
  list.go
  map.go
  persistent_vector.go
  seq.go
  set.go
  vector.go
)
for file in "${moved_collection_files[@]}"; do
  if [[ -e "core/${file}" ]]; then
    fail "collection-owned file core/${file} has moved to core/types/collections; do not reintroduce root copies"
  fi
done
if [[ -e core/chunked_procs.go ]]; then
  fail "chunk proc registration has been coalesced into core/runtime_kernel.go; do not reintroduce standalone core/chunked_procs.go"
fi

moved_runtime_files=(
  channel.go
)
for file in "${moved_runtime_files[@]}"; do
  if [[ -e "core/${file}" ]]; then
    fail "runtime-owned file core/${file} has moved to core/runtime; do not reintroduce root copies"
  fi
done
if grep -R '^type \(Future\|Promise\|Agent\|Atom\) struct' core --include='*.go' --exclude-dir=runtime >/dev/null; then
  grep -R '^type \(Future\|Promise\|Agent\|Atom\) struct' core --include='*.go' --exclude-dir=runtime >&2
  fail "Future/Promise/Agent/Atom object wrappers are runtime-owned; do not reintroduce root copies"
fi
if grep -R -E 'var atom_NUM_[0-9]+ Atom|Atom = Atom\{|\*Atom\)\(nil\)' core --include='*.go' --exclude-dir=runtime >/dev/null; then
  grep -R -E 'var atom_NUM_[0-9]+ Atom|Atom = Atom\{|\*Atom\)\(nil\)' core --include='*.go' --exclude-dir=runtime >&2
  fail "generated/root code must use corert.Atom and corert.NewAtom, not root Atom literals or reflect references"
fi

for pkg in runtime reader types/collections types/string types/numerical; do
  if grep -R 'github.com/rcarmo/go-joker/core"' "core/${pkg}" --include='*.go' >/dev/null; then
    fail "core/${pkg} must not import root core; define an adapter contract before moving coupled code"
  fi
done
if grep -R 'github.com/rcarmo/go-joker/core/runtime' core/types --include='*.go' >/dev/null; then
  grep -R 'github.com/rcarmo/go-joker/core/runtime' core/types --include='*.go' >&2
  fail "core/types must not import core/runtime; runtime object wrappers own runtime-dependent behavior"
fi

for artifact in core.test joker transit.test; do
  if [[ -e "$artifact" ]]; then
    fail "stale root build artifact present: $artifact"
  fi
done

if ! grep -qx 'module github.com/rcarmo/go-joker' go.mod; then
  fail "go.mod module path is not github.com/rcarmo/go-joker"
fi

exit "$status"
