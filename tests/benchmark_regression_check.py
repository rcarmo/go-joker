#!/usr/bin/env python3
"""Apply conservative regression thresholds to two Go benchmark result files."""

from __future__ import annotations

import argparse
import re
import statistics
import sys
from pathlib import Path

LINE_RE = re.compile(
    r"^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+"
    r"(?P<ns>\d+(?:\.\d+)?) ns/op"
    r"(?:\s+(?P<bytes>\d+) B/op)?"
    r"(?:\s+(?P<allocs>\d+) allocs/op)?$"
)


def parse(path: Path) -> dict[str, dict[str, list[float]]]:
    samples: dict[str, dict[str, list[float]]] = {}
    for raw in path.read_text().splitlines():
        match = LINE_RE.match(raw.strip())
        if not match:
            continue
        values = samples.setdefault(match.group(1), {"ns": [], "bytes": [], "allocs": []})
        values["ns"].append(float(match.group("ns")))
        if match.group("bytes") is not None:
            values["bytes"].append(float(match.group("bytes")))
        if match.group("allocs") is not None:
            values["allocs"].append(float(match.group("allocs")))
    return samples


def cv(values: list[float]) -> float:
    median = statistics.median(values)
    return 0.0 if len(values) < 2 or median == 0 else statistics.stdev(values) / median


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("baseline", type=Path)
    parser.add_argument("candidate", type=Path)
    parser.add_argument("--min-samples", type=int, default=6)
    parser.add_argument("--max-cv", type=float, default=0.10)
    parser.add_argument("--time-regression", type=float, default=0.15)
    parser.add_argument("--bytes-regression", type=float, default=0.05)
    args = parser.parse_args()

    baseline = parse(args.baseline)
    candidate = parse(args.candidate)
    failures: list[str] = []
    if set(baseline) != set(candidate):
        missing = sorted(set(baseline) - set(candidate))
        added = sorted(set(candidate) - set(baseline))
        print(f"benchmark set mismatch: missing={missing} added={added}", file=sys.stderr)
        return 1

    print("benchmark regression policy:")
    for name in sorted(baseline):
        old, new = baseline[name], candidate[name]
        if len(old["ns"]) < args.min_samples or len(new["ns"]) < args.min_samples:
            failures.append(f"{name}: requires at least {args.min_samples} samples per side")
            continue
        old_ns, new_ns = statistics.median(old["ns"]), statistics.median(new["ns"])
        delta = new_ns / old_ns - 1.0
        stable = cv(old["ns"]) <= args.max_cv and cv(new["ns"]) <= args.max_cv
        verdict = "stable" if stable else "timing skipped (noisy)"
        print(f"  {name}: {old_ns:.2f} -> {new_ns:.2f} ns/op ({delta:+.1%}); {verdict}")
        if stable and delta > args.time_regression:
            failures.append(f"{name}: stable time regression {delta:+.1%} exceeds {args.time_regression:.0%}")

        if old["bytes"] and new["bytes"]:
            old_bytes, new_bytes = statistics.median(old["bytes"]), statistics.median(new["bytes"])
            # Ignore allocator rounding below one machine word, but fail material growth.
            bytes_stable = cv(old["bytes"]) <= args.max_cv and cv(new["bytes"]) <= args.max_cv
            if bytes_stable and new_bytes - old_bytes >= 8 and new_bytes > old_bytes * (1 + args.bytes_regression):
                failures.append(f"{name}: stable B/op increased from {old_bytes:g} to {new_bytes:g}")
        if old["allocs"] and new["allocs"]:
            old_allocs, new_allocs = statistics.median(old["allocs"]), statistics.median(new["allocs"])
            if new_allocs > old_allocs:
                failures.append(f"{name}: allocs/op increased from {old_allocs:g} to {new_allocs:g}")

    if failures:
        print("regression check failed:", file=sys.stderr)
        for failure in failures:
            print(f"  - {failure}", file=sys.stderr)
        return 1
    print("benchmark regression check passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
