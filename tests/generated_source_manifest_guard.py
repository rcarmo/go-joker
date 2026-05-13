#!/usr/bin/env python3
import pathlib
import re
import sys

root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else ".")
gen_code = root / "core" / "gen" / "codegen" / "main.go"
manifest = root / "core" / "generated" / "core_sources_gen.go"

src = gen_code.read_text()
try:
    block = src.split("var CoreSourceFiles []FileInfo = []FileInfo{", 1)[1].split("\n}\n", 1)[0]
except IndexError:
    print("generated source manifest guard: cannot locate CoreSourceFiles block", file=sys.stderr)
    sys.exit(1)

entries = re.findall(r'Name:\s+"([^"]+)",\s*Filename:\s+"([^"]+)"', block)
expected = []
for name, filename in entries:
    if not (name.startswith("<") and name.endswith(">")):
        print(f"generated source manifest guard: unexpected core source name syntax: {name}", file=sys.stderr)
        sys.exit(1)
    expected.append((name[1:-1], filename))

manifest_src = manifest.read_text()
actual = re.findall(r'\{Name:\s+"([^"]+)",\s+Path:\s+"([^"]+)"\}', manifest_src)

if actual != expected:
    print("generated source manifest guard: core_sources_gen.go is not in sync with CoreSourceFiles", file=sys.stderr)
    print("expected:", expected, file=sys.stderr)
    print("actual:  ", actual, file=sys.stderr)
    sys.exit(1)
