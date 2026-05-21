#!/usr/bin/env python3
"""Report/guard the final coalesced root core layout.

The historical mass coalescing passes have been applied and committed.  This
helper is intentionally idempotent now: it documents the allowed handwritten
root files and fails if a former split-out root file is reintroduced instead of
being moved to a real package or coalesced into the runtime kernel.
"""
from __future__ import annotations

from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
CORE = ROOT / "core"

ALLOWED_ROOT_CORE = {
    "a_generated_bootstrap_payloads.go",
    "bootstrap_gen_code.go",
    "runtime_kernel.go",
    "runtime_kernel_contracts_test.go",
    "runtime_kernel_spew_enabled.go",
}

# Files removed during the root coalescing recovery.  They should not come back
# as root `package core` files; use real package ownership or the kernel files.
COALESCED_ROOT_FILES = {
    "array_map.go",
    "array_vector.go",
    "assert_root.go",
    "boxed_exec.go",
    "channel.go",
    "chunked_procs.go",
    "chunked_seq.go",
    "code.go",
    "common.go",
    "concurrency_ext.go",
    "core_api_gaps.go",
    "environment.go",
    "environment_fast_init.go",
    "environment_slow_init.go",
    "eval.go",
    "expr.go",
    "fn_ir_cache.go",
    "format.go",
    "function_trace.go",
    "goroutine_rt.go",
    "hash_map.go",
    "hierarchy.go",
    "hierarchy_init.go",
    "io_objects.go",
    "list.go",
    "loop_compiler.go",
    "map.go",
    "native_recursive.go",
    "ns.go",
    "object.go",
    "object_slow_init.go",
    "object_spew_enabled.go",
    "pack.go",
    "parse.go",
    "parse_slow_init.go",
    "persistent_vector.go",
    "procs.go",
    "procs_slow_init.go",
    "protocol.go",
    "protocol_init.go",
    "public_forms.go",
    "read.go",
    "reader_construction.go",
    "record.go",
    "record_init.go",
    "reduce_fast.go",
    "reduced.go",
    "root_object_support.go",
    "runtime_execution_contract.go",
    "seq.go",
    "set.go",
    "sorted_colls.go",
    "spew_disabled.go",
    "spew_enabled.go",
    "string_cursor.go",
    "string_objects.go",
    "string_runtime.go",
    "tagged_literals.go",
    "tail_call.go",
    "tco.go",
    "tco_rewrite.go",
    "transducer_compat.go",
    "transient.go",
    "unchecked_arith.go",
    "vector.go",
    "wasm_compile.go",
    "with_info_root.go",
    "z_doc_meta.go",
}


def main() -> int:
    files = sorted(p.name for p in CORE.glob("*.go"))
    unexpected = [name for name in files if name not in ALLOWED_ROOT_CORE]
    missing = sorted(ALLOWED_ROOT_CORE - set(files))
    resurrected = sorted(name for name in COALESCED_ROOT_FILES if (CORE / name).exists())

    print("root core files:")
    for name in files:
        print(f"  {name}")
    print(f"count: {len(files)}")

    if missing:
        print("missing expected root files: " + ", ".join(missing), file=sys.stderr)
    if unexpected:
        print("unexpected root files: " + ", ".join(unexpected), file=sys.stderr)
    if resurrected:
        print("coalesced files reintroduced: " + ", ".join(resurrected), file=sys.stderr)
    return 1 if missing or unexpected or resurrected else 0


if __name__ == "__main__":
    raise SystemExit(main())
