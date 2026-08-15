#!/usr/bin/env python3
"""Summarize a run-comparison.sh output directory into a comparison table.

Reads probe/run-N.json + probe/run-N.wallclock_ms and maestro/run-N.xml +
maestro/run-N.wallclock_ms, and prints median/p90 wall-clock time and flake
rate for each tool. Standard-library only — no extra dependencies.

Usage: summarize.py OUT_DIR RUNS
"""
import json
import statistics
import sys

# xml.etree.ElementTree, not defusedxml: the XML parsed here is generated
# entirely locally by the `maestro` CLI this script itself invokes, never
# from an untrusted/network source, and modern CPython's ElementTree
# (3.7.1+) already ignores external entities by default. Pulling in a
# non-stdlib dependency for a threat model that doesn't apply here would
# undercut this script's "standard library only, zero setup" design goal.
import xml.etree.ElementTree as ET
from pathlib import Path


def percentile(values, pct):
    if not values:
        return 0.0
    ordered = sorted(values)
    k = (len(ordered) - 1) * pct
    f = int(k)
    c = min(f + 1, len(ordered) - 1)
    if f == c:
        return ordered[f]
    return ordered[f] + (ordered[c] - ordered[f]) * (k - f)


def read_wallclock(path):
    try:
        return int(path.read_text().strip())
    except (FileNotFoundError, ValueError):
        return None


def summarize_probe(out_dir, runs):
    times_ms = []
    clean_runs = 0
    for i in range(1, runs + 1):
        wc = read_wallclock(out_dir / "probe" / f"run-{i}.wallclock_ms")
        if wc is not None:
            times_ms.append(wc)
        report_path = out_dir / "probe" / f"run-{i}.json"
        try:
            report = json.loads(report_path.read_text())
            if report.get("failed", 1) == 0 and report.get("total_tests", 0) > 0:
                clean_runs += 1
        except (FileNotFoundError, json.JSONDecodeError):
            pass  # non-clean run; report missing or malformed counts as a flake
    return times_ms, clean_runs


def summarize_maestro(out_dir, runs):
    times_ms = []
    clean_runs = 0
    for i in range(1, runs + 1):
        wc = read_wallclock(out_dir / "maestro" / f"run-{i}.wallclock_ms")
        if wc is not None:
            times_ms.append(wc)
        xml_path = out_dir / "maestro" / f"run-{i}.xml"
        try:
            root = ET.parse(xml_path).getroot()
            # JUnit: either a single <testsuite> or a <testsuites> wrapper.
            suites = [root] if root.tag == "testsuite" else list(root.findall("testsuite"))
            failures = sum(int(s.get("failures", 0)) + int(s.get("errors", 0)) for s in suites)
            total = sum(int(s.get("tests", 0)) for s in suites)
            if failures == 0 and total > 0:
                clean_runs += 1
        except (FileNotFoundError, ET.ParseError):
            pass  # non-clean run; report missing or malformed counts as a flake
    return times_ms, clean_runs


def fmt_ms(ms):
    return f"{ms / 1000:.1f}s"


def main():
    if len(sys.argv) != 3:
        print(__doc__, file=sys.stderr)
        sys.exit(1)
    out_dir = Path(sys.argv[1])
    runs = int(sys.argv[2])

    probe_times, probe_clean = summarize_probe(out_dir, runs)
    maestro_times, maestro_clean = summarize_maestro(out_dir, runs)

    rows = [
        ("Tool", "Runs", "Median", "P90", "Flake rate"),
        ("-" * 10, "-" * 6, "-" * 8, "-" * 8, "-" * 12),
    ]
    for name, times, clean in (
        ("FlutterProbe", probe_times, probe_clean),
        ("Maestro", maestro_times, maestro_clean),
    ):
        if times:
            median = fmt_ms(statistics.median(times))
            p90 = fmt_ms(percentile(times, 0.90))
        else:
            median = p90 = "n/a"
        flake_pct = 0.0 if runs == 0 else (1 - clean / runs) * 100
        rows.append((name, str(len(times)), median, p90, f"{flake_pct:.0f}%"))

    widths = [max(len(r[c]) for r in rows) for c in range(len(rows[0]))]
    for row in rows:
        print("  ".join(cell.ljust(w) for cell, w in zip(row, widths)))

    print()
    print(f"Raw per-run data: {out_dir}/probe/*.wallclock_ms + *.json, "
          f"{out_dir}/maestro/*.wallclock_ms + *.xml")


if __name__ == "__main__":
    main()
