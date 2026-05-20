#!/usr/bin/env python3
"""Coalesce small root core files into owning domain files."""
from __future__ import annotations
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
CORE = ROOT / "core"

def strip_package_and_imports(path: Path) -> str:
    src = path.read_text(); lines = src.splitlines(True)
    if not lines or not lines[0].startswith("package "):
        raise SystemExit(f"{path}: expected package declaration on first line")
    rest = "".join(lines[1:]).lstrip("\n")
    if rest.startswith("import ("):
        rest = rest[rest.index("\n)") + 3:].lstrip("\n")
    else:
        rest = re.sub(r'^import\s+[^\n]+\n\n?', '', rest, count=1)
    return rest.rstrip() + "\n"

def append_body(src_name: str, dst_name: str, marker: str | None = None) -> bool:
    src = CORE / src_name; dst = CORE / dst_name
    if not src.exists():
        print(f"skip missing {src_name}"); return False
    marker = marker or f"// ---- {src_name} ----"
    text = dst.read_text().rstrip()
    if marker not in text:
        dst.write_text(text + "\n\n" + marker + "\n" + strip_package_and_imports(src))
        print(f"merged {src_name} -> {dst_name}")
    else:
        print(f"skip already merged {src_name} -> {dst_name}")
    src.unlink(); return True

def ensure_import(dst_name: str, spec: str) -> None:
    path = CORE / dst_name
    if not path.exists(): return
    src = path.read_text()
    if spec in src: return
    if "import (\n" not in src: raise SystemExit(f"{dst_name}: expected import block")
    path.write_text(src.replace("import (\n", "import (\n\t" + spec + "\n", 1))

def rename_file(old_name: str, new_name: str) -> None:
    old = CORE / old_name; new = CORE / new_name
    if new.exists():
        print(f"skip rename {old_name}; {new_name} exists")
        if old.exists(): raise SystemExit(f"both {old_name} and {new_name} exist")
        return
    if not old.exists():
        print(f"skip missing {old_name}"); return
    old.rename(new); print(f"renamed {old_name} -> {new_name}")

def first_pass() -> None:
    if append_body("string_cursor.go", "string_objects.go"):
        ensure_import("string_objects.go", '"sync"')
    rename_file("string_objects.go", "string_runtime.go")
    if append_body("hierarchy_init.go", "hierarchy.go"):
        ensure_import("hierarchy.go", 'corecollections "github.com/rcarmo/go-joker/core/types/collections"')
    append_body("with_info_root.go", "assert_root.go")
    rename_file("assert_root.go", "root_object_support.go")
    append_body("reduced.go", "transducer_compat.go")

def repeat_pass() -> None:
    if append_body("z_doc_meta.go", "ns.go"):
        ensure_import("ns.go", 'corecollections "github.com/rcarmo/go-joker/core/types/collections"')
    if append_body("goroutine_rt.go", "environment.go"):
        ensure_import("environment.go", '"sync"')
        ensure_import("environment.go", 'corert "github.com/rcarmo/go-joker/core/runtime"')
    append_body("io_objects.go", "procs.go")


def third_pass() -> None:
    # TCO runtime trampoline and parse-time recur rewrite are one optimization domain.
    append_body("tco_rewrite.go", "tco.go")
    rename_file("tco.go", "tail_call.go")

    # Function tracing is call/proc instrumentation; co-locate with core call mechanics.
    append_body("function_trace.go", "object.go")
    for spec in [
        '"fmt"',
        '"time"',
        'coreir "github.com/rcarmo/go-joker/core/ir"',
        'coretrace "github.com/rcarmo/go-joker/core/trace"',
    ]:
        ensure_import("object.go", spec)



def fourth_pass() -> None:
    # Core API compatibility gaps and unchecked arithmetic are root proc registrations; co-locate with procs.
    if append_body("core_api_gaps.go", "procs.go"):
        ensure_import("procs.go", '"path/filepath"')
    append_body("unchecked_arith.go", "procs.go")

    # Tagged literal reader registration belongs with reader/runtime construction until reader ownership moves.
    append_body("tagged_literals.go", "read.go")



def fifth_pass() -> None:
    # Sorted collections and transient bridges are collection/proc glue; root keeps registration only until proc/env moves.
    append_body("sorted_colls.go", "procs.go")
    append_body("transient.go", "procs.go")



def sixth_pass() -> None:
    # Concurrency and core.async namespace glue are root proc/env registrations; co-locate with procs.
    if append_body("concurrency_ext.go", "procs.go"):
        ensure_import("procs.go", '"reflect"')
    if append_body("core_async_ext.go", "procs.go"):
        ensure_import("procs.go", '"unsafe"')
        ensure_import("procs.go", '"github.com/rcarmo/go-joker/core/hashutil"')



def seventh_pass() -> None:
    # Misc root runtime support: exit hooks, formatting callbacks, file-info maps, truthiness.
    if append_body("common.go", "object.go"):
        for spec in [
            '"io"',
            '"math/big"',
            '"os"',
            '"unicode/utf8"',
        ]:
            ensure_import("object.go", spec)



def eighth_pass() -> None:
    # Root object assertions/WithInfo and root-owned string wrappers belong with object mechanics.
    append_body("root_object_support.go", "object.go")
    if append_body("string_runtime.go", "object.go"):
        ensure_import("object.go", '"sync"')

    # Tail-call evaluation/rewrite is evaluator machinery.
    append_body("tail_call.go", "eval.go")


def main() -> None:
    first_pass(); repeat_pass(); third_pass(); fourth_pass(); fifth_pass(); sixth_pass(); seventh_pass(); eighth_pass()

if __name__ == "__main__": main()
