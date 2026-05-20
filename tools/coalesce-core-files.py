#!/usr/bin/env python3
"""Coalesce small root core files into owning domain files."""
from __future__ import annotations
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
CORE = ROOT / "core"

def strip_package_and_imports(path: Path) -> str:
    src = path.read_text(); lines = src.splitlines(True)
    pkg_idx = next((i for i, line in enumerate(lines) if line.startswith("package ")), -1)
    if pkg_idx < 0:
        raise SystemExit(f"{path}: expected package declaration")
    rest = "".join(lines[pkg_idx + 1:]).lstrip("\n")
    # Drop the first import block or single-line import even when file comments sit
    # between the package declaration and imports.
    rest = re.sub(r'\nimport \(\n.*?\n\)\n', '\n', rest, count=1, flags=re.S)
    rest = re.sub(r'^import \(\n.*?\n\)\n', '', rest, count=1, flags=re.S)
    rest = re.sub(r'\nimport\s+[^\n]+\n', '\n', rest, count=1)
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



def ninth_pass() -> None:
    # Reader construction adapter belongs with the root reader implementation.
    append_body("reader_construction.go", "read.go")

    # Protocols, records, and hierarchies form one Clojure parity/type-extension domain.
    if append_body("record.go", "protocol.go"):
        ensure_import("protocol.go", '"strings"')
    append_body("hierarchy.go", "protocol.go")



def tenth_pass() -> None:
    # gen_code-only slow bootstrap helpers are one generator/bootstrap domain.
    if append_body("parse_slow_init.go", "code.go"):
        pass
    if append_body("procs_slow_init.go", "code.go"):
        ensure_import("code.go", 'corecollections "github.com/rcarmo/go-joker/core/types/collections"')
    if append_body("object_slow_init.go", "code.go"):
        ensure_import("code.go", '"io"')
        ensure_import("code.go", 'corert "github.com/rcarmo/go-joker/core/runtime"')
        ensure_import("code.go", 'corecollections "github.com/rcarmo/go-joker/core/types/collections"')



def eleventh_pass() -> None:
    # Formatting callbacks are object/runtime presentation mechanics.
    if append_body("format.go", "object.go"):
        ensure_import("object.go", '"regexp"')
        ensure_import("object.go", '"sort"')

    # Reduced/transducer compatibility belongs with reduce fast paths.
    append_body("transducer_compat.go", "reduce_fast.go")

    # Runtime execution adapter belongs beside boxed executor glue until IR package extraction.
    append_body("runtime_execution_contract.go", "boxed_exec.go")

    # Native recursive specialization is part of fn IR cache/compilation ownership.
    append_body("native_recursive.go", "fn_ir_cache.go")



def twelfth_pass() -> None:
    # Expression dump/pack helpers are evaluator/expression mechanics.
    if append_body("expr.go", "eval.go"):
        pass
    if append_body("pack.go", "eval.go"):
        for spec in ['"bytes"', '"encoding/binary"', '"maps"', '"slices"']:
            ensure_import("eval.go", spec)

    # IR compiler and WASM lowering are one executor/compiler cluster until moved to core/ir+core/wasm.
    if append_body("loop_compiler.go", "fn_ir_cache.go"):
        ensure_import("fn_ir_cache.go", '"fmt"')
        ensure_import("fn_ir_cache.go", '"math"')
        ensure_import("fn_ir_cache.go", 'corert "github.com/rcarmo/go-joker/core/runtime"')
        ensure_import("fn_ir_cache.go", 'corecollections "github.com/rcarmo/go-joker/core/types/collections"')
    if append_body("wasm_compile.go", "fn_ir_cache.go"):
        for spec in ['"context"', '"encoding/binary"', '"math"', '"reflect"']:
            ensure_import("fn_ir_cache.go", spec)
        ensure_import("fn_ir_cache.go", 'corewasm "github.com/rcarmo/go-joker/core/wasm"')



def thirteenth_pass() -> None:
    # Namespace state belongs with Env until runtime/env package extraction.
    if append_body("ns.go", "environment.go"):
        ensure_import("environment.go", '"fmt"')

    # Reader implementation and parser share root expression/reader construction ownership.
    if append_body("read.go", "parse.go"):
        for spec in ['"io"', '"math/big"', '"math/rand"', '"strconv"']:
            ensure_import("parse.go", spec)
        ensure_import("parse.go", '"github.com/rcarmo/go-joker/core/types/numerical"')

    # gen_code Env construction belongs with the rest of gen_code bootstrap helpers.
    if append_body("environment_slow_init.go", "code.go"):
        ensure_import("code.go", 'corecollections "github.com/rcarmo/go-joker/core/types/collections"')



def fourteenth_pass() -> None:
    # Boxed executor belongs with the IR compile/cache/execution cluster until core/ir owns it.
    if append_body("boxed_exec.go", "fn_ir_cache.go"):
        for spec in ['"strconv"', '"unicode/utf8"', '"unsafe"']:
            ensure_import("fn_ir_cache.go", spec)
        ensure_import("fn_ir_cache.go", 'coreirx "github.com/rcarmo/go-joker/core/ir"')

    # Reduce/transducer fast paths are evaluator execution mechanics.
    if append_body("reduce_fast.go", "eval.go"):
        ensure_import("eval.go", '"sync"')



def fifteenth_pass() -> None:
    # Root object model now owns env/namespace and protocol/record/hierarchy runtime glue.
    if append_body("environment.go", "object.go"):
        ensure_import("object.go", '"github.com/rcarmo/go-joker/core/osutil"')
    if append_body("protocol.go", "object.go"):
        ensure_import("object.go", '"strings"')

    # Parser/reader integration belongs with evaluator front-end until a true package move.
    if append_body("parse.go", "eval.go"):
        for spec in ['"io"', '"math/big"', '"math/rand"', '"regexp"', '"strconv"']:
            ensure_import("eval.go", spec)
        ensure_import("eval.go", 'corereader "github.com/rcarmo/go-joker/core/reader"')
        ensure_import("eval.go", '"github.com/rcarmo/go-joker/core/types/numerical"')



def sixteenth_pass() -> None:
    # Root proc registration/call helpers are object/runtime glue until proc/env extraction.
    if append_body("procs.go", "object.go"):
        for spec in [
            '"bytes"', '"errors"', '"math/rand"', '"path/filepath"',
            'coregenerated "github.com/rcarmo/go-joker/core/generated"',
            'corereader "github.com/rcarmo/go-joker/core/reader"',
            '"github.com/rcarmo/go-joker/core/deps"',
            '"github.com/rcarmo/go-joker/core/types/numerical"',
            '"github.com/tetratelabs/wazero/api"',
        ]:
            ensure_import("object.go", spec)


def main() -> None:
    first_pass(); repeat_pass(); third_pass(); fourth_pass(); fifth_pass(); sixth_pass(); seventh_pass(); eighth_pass(); ninth_pass(); tenth_pass(); eleventh_pass(); twelfth_pass(); thirteenth_pass(); fourteenth_pass(); fifteenth_pass(); sixteenth_pass()

if __name__ == "__main__": main()
