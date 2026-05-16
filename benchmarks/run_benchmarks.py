#!/usr/bin/env python3
"""Run Go benchmarks repeatedly and summarize medians.

This is intentionally dependency-free. It separates the noisy `go test` output
from the decision data we care about: median ns/op, allocs/op and bytes/op.

Examples:
  python3 benchmarks/run_benchmarks.py --runs 7 --bench 'BenchmarkCLBG|BenchmarkEval'
  python3 benchmarks/run_benchmarks.py --runs 5 --bench 'BenchmarkIRString|BenchmarkIRChar' --json /tmp/ir.json
"""

from __future__ import annotations

import argparse
import json
import re
import statistics
import subprocess
import sys
import time
from dataclasses import dataclass, asdict
from pathlib import Path
from typing import Dict, List

BENCH_RE = re.compile(
    r"^(Benchmark\S+)-\d+\s+\d+\s+"
    r"(?P<ns>\d+(?:\.\d+)?) ns/op"
    r"(?:\s+(?P<bytes>\d+) B/op)?"
    r"(?:\s+(?P<allocs>\d+) allocs/op)?"
)


@dataclass
class Sample:
    ns_per_op: float
    bytes_per_op: int | None = None
    allocs_per_op: int | None = None


@dataclass
class Summary:
    benchmark: str
    runs: int
    median_ns_per_op: float
    min_ns_per_op: int
    max_ns_per_op: int
    stdev_pct: float
    median_ms_per_op: float
    median_bytes_per_op: float | None
    median_allocs_per_op: float | None


def parse_output(output: str) -> Dict[str, Sample]:
    parsed: Dict[str, Sample] = {}
    for line in output.splitlines():
        m = BENCH_RE.match(line.strip())
        if not m:
            continue
        name = m.group(1)
        parsed[name] = Sample(
            ns_per_op=float(m.group("ns")),
            bytes_per_op=int(m.group("bytes")) if m.group("bytes") else None,
            allocs_per_op=int(m.group("allocs")) if m.group("allocs") else None,
        )
    return parsed


def median_optional(values: List[int | None]) -> float | None:
    clean = [v for v in values if v is not None]
    if not clean:
        return None
    return float(statistics.median(clean))


def summarize(samples: Dict[str, List[Sample]]) -> List[Summary]:
    out: List[Summary] = []
    for name in sorted(samples):
        vals = samples[name]
        ns = [float(s.ns_per_op) for s in vals]
        med = float(statistics.median(ns))
        stdev_pct = 0.0
        if len(ns) > 1 and med:
            stdev_pct = statistics.stdev(ns) / med * 100.0
        out.append(
            Summary(
                benchmark=name,
                runs=len(vals),
                median_ns_per_op=med,
                min_ns_per_op=min(ns),
                max_ns_per_op=max(ns),
                stdev_pct=stdev_pct,
                median_ms_per_op=med / 1_000_000.0,
                median_bytes_per_op=median_optional([s.bytes_per_op for s in vals]),
                median_allocs_per_op=median_optional([s.allocs_per_op for s in vals]),
            )
        )
    return out


def print_markdown(summaries: List[Summary]) -> None:
    print("| Benchmark | runs | median ms/op | min ms | max ms | stdev % | B/op | allocs/op |")
    print("|---|---:|---:|---:|---:|---:|---:|---:|")
    for s in summaries:
        b = "" if s.median_bytes_per_op is None else f"{s.median_bytes_per_op:.0f}"
        a = "" if s.median_allocs_per_op is None else f"{s.median_allocs_per_op:.0f}"
        print(
            f"| `{s.benchmark}` | {s.runs} | {s.median_ms_per_op:.3f} | "
            f"{s.min_ns_per_op/1_000_000:.3f} | {s.max_ns_per_op/1_000_000:.3f} | "
            f"{s.stdev_pct:.1f} | {b} | {a} |"
        )


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--runs", type=int, default=7)
    ap.add_argument("--bench", default="BenchmarkCLBG|BenchmarkEval|BenchmarkWasm")
    ap.add_argument("--benchtime", default="5x")
    ap.add_argument("--timeout", default="600s")
    ap.add_argument("--pkg", default="./core")
    ap.add_argument("--json", type=Path)
    ap.add_argument("--raw-dir", type=Path)
    ap.add_argument("--env", action="append", default=[], help="KEY=VALUE passed to go test")
    args = ap.parse_args()

    env = None
    if args.env:
        import os

        env = os.environ.copy()
        for item in args.env:
            k, _, v = item.partition("=")
            env[k] = v

    samples: Dict[str, List[Sample]] = {}
    raw_outputs: List[str] = []
    cmd = [
        "go",
        "test",
        args.pkg,
        "-run",
        "^$",
        "-bench",
        args.bench,
        "-benchmem",
        f"-benchtime={args.benchtime}",
        f"-timeout={args.timeout}",
    ]
    for i in range(args.runs):
        print(f"run {i+1}/{args.runs}: {' '.join(cmd)}", file=sys.stderr)
        start = time.time()
        p = subprocess.run(cmd, text=True, capture_output=True, env=env)
        elapsed = time.time() - start
        raw = p.stdout + p.stderr
        raw_outputs.append(raw)
        if p.returncode != 0:
            print(raw, file=sys.stderr)
            return p.returncode
        parsed = parse_output(raw)
        print(f"  parsed {len(parsed)} benchmarks in {elapsed:.1f}s", file=sys.stderr)
        for name, sample in parsed.items():
            samples.setdefault(name, []).append(sample)

    missing_runs = {name: len(vals) for name, vals in samples.items() if len(vals) != args.runs}
    if missing_runs:
        print(f"missing benchmark samples: {missing_runs}", file=sys.stderr)
        return 1
    if not samples:
        print("no benchmark samples parsed", file=sys.stderr)
        return 1

    summaries = summarize(samples)
    print_markdown(summaries)

    payload = {
        "command": cmd,
        "runs_requested": args.runs,
        "bench": args.bench,
        "benchtime": args.benchtime,
        "summaries": [asdict(s) for s in summaries],
        "samples": {k: [asdict(s) for s in v] for k, v in samples.items()},
    }
    if args.json:
        args.json.parent.mkdir(parents=True, exist_ok=True)
        args.json.write_text(json.dumps(payload, indent=2))
    if args.raw_dir:
        args.raw_dir.mkdir(parents=True, exist_ok=True)
        for i, raw in enumerate(raw_outputs, 1):
            (args.raw_dir / f"run-{i:02d}.txt").write_text(raw)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
