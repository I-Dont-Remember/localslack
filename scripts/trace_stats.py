#!/usr/bin/env python3
"""Efficiency stats for a Claude Code session trace (+ its subagents).

Usage: trace_stats.py <session.jsonl | session-id> [--compare <other>] [--json]
Session ids resolve under ~/.claude/projects/*/<id>.jsonl. See
agent-docs/reviews/2026-10-13-agent-trace-review.md for what the numbers mean.
"""

import argparse
import itertools
import json
import math
import re
import sys
from collections import Counter
from datetime import datetime
from pathlib import Path

IDLE_GAP_SECONDS = 30 * 60
BASH_EDIT = re.compile(
    r"\bsed\s+(-\w*i\w*)\b|\bpython3?\s+-\s*<<|\bcat\s+>>?\s*\S+|\btee\s+\S+\.(go|py|md)"
)
SCRATCH = re.compile(r"(^|[\s/])(tmp/|\$TMPDIR|/tmp/)")
GUARD_MARKERS = ("too complex to verify", "Refusing to run")
SUITE = re.compile(r"\bmake (check|test|verify)(?![\w-])")


def _ts(row) -> datetime | None:
    raw = row.get("timestamp")
    return datetime.fromisoformat(raw) if raw else None


def _rows(path: Path):
    for line in path.read_text().splitlines():
        try:
            yield json.loads(line)
        except json.JSONDecodeError:
            continue


def _text(content) -> str:
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "".join(b.get("text", "") for b in content if isinstance(b, dict))
    return ""


def _pct(values: list[int], p: float) -> int:
    if not values:
        return 0
    ordered = sorted(values)
    return ordered[max(0, math.ceil(p * len(ordered)) - 1)]


def _is_bash_edit(cmd: str) -> bool:
    return bool(BASH_EDIT.search(cmd)) and not SCRATCH.search(cmd)


def _max_concurrency(spans: list[tuple[datetime, datetime]]) -> int:
    events = sorted(
        [(s, 1) for s, _ in spans] + [(e, -1) for _, e in spans],
        key=lambda x: (x[0], x[1]),
    )
    cur = best = 0
    for _, delta in events:
        cur += delta
        best = max(best, cur)
    return best


def compute(main: Path) -> dict:
    sub_dir = main.with_suffix("") / "subagents"
    subs = sorted(sub_dir.glob("*.jsonl")) if sub_dir.is_dir() else []
    tools: Counter = Counter()
    suite: Counter = Counter({"make check": 0, "make test": 0, "make verify": 0})
    tot = Counter()
    contexts: list[int] = []
    result_bytes: list[int] = []
    stamps: list[datetime] = []
    spans: list[tuple[datetime, datetime]] = []
    for path in [main, *subs]:
        file_stamps = []
        for row in _rows(path):
            ts = _ts(row)
            if ts:
                stamps.append(ts)
                file_stamps.append(ts)
            msg = row.get("message") or {}
            if row.get("type") == "assistant":
                tot["turns"] += 1
                if row.get("isApiErrorMessage") and "session limit" in _text(
                    msg.get("content")
                ):
                    tot["session_limit_hits"] += 1
                usage = msg.get("usage") or {}
                if usage:
                    tot["output_tokens"] += usage.get("output_tokens", 0)
                    tot["cache_read_tokens"] += usage.get("cache_read_input_tokens", 0)
                    tot["cache_write_tokens"] += usage.get(
                        "cache_creation_input_tokens", 0
                    )
                    contexts.append(usage.get("cache_read_input_tokens", 0))
                for block in msg.get("content") or []:
                    if not isinstance(block, dict) or block.get("type") != "tool_use":
                        continue
                    tools[block["name"]] += 1
                    cmd = (block.get("input") or {}).get("command", "")
                    if block["name"].lower() == "bash":
                        tot["bash_edit_calls"] += _is_bash_edit(cmd)
                        for m in SUITE.finditer(cmd):
                            suite[f"make {m.group(1)}"] += 1
            elif row.get("type") == "user" and isinstance(msg.get("content"), list):
                for block in msg["content"]:
                    if isinstance(block, dict) and block.get("type") == "tool_result":
                        text = _text(block.get("content"))
                        result_bytes.append(len(text))
                        tot["guard_refusals"] += any(m in text for m in GUARD_MARKERS)
        if path != main and file_stamps:
            spans.append((min(file_stamps), max(file_stamps)))
    stamps.sort()
    idle = sum(
        (b - a).total_seconds()
        for a, b in itertools.pairwise(stamps)
        if (b - a).total_seconds() > IDLE_GAP_SECONDS
    )
    return {
        "turns": tot["turns"],
        "subagents": len(subs),
        "max_concurrency": _max_concurrency(spans),
        "tool_calls": dict(tools),
        "bash_edit_calls": tot["bash_edit_calls"],
        "guard_refusals": tot["guard_refusals"],
        "session_limit_hits": tot["session_limit_hits"],
        "full_suite_runs": dict(suite),
        "output_tokens": tot["output_tokens"],
        "cache_read_tokens": tot["cache_read_tokens"],
        "cache_write_tokens": tot["cache_write_tokens"],
        "peak_context": max(contexts, default=0),
        "context_p50": _pct(contexts, 0.5),
        "context_p95": _pct(contexts, 0.95),
        "result_bytes_p50": _pct(result_bytes, 0.5),
        "result_bytes_p95": _pct(result_bytes, 0.95),
        "idle_hours": round(idle / 3600, 1),
    }


def compare(a: dict, b: dict) -> dict:
    out = {}
    for key, va in a.items():
        vb = b.get(key)
        if isinstance(va, (int, float)) and isinstance(vb, (int, float)):
            out[key] = (va, vb, round((vb - va) / va * 100, 1) if va else 0.0)
    return out


def resolve(arg: str) -> Path:
    path = Path(arg)
    if path.is_file():
        return path
    hits = list((Path.home() / ".claude" / "projects").glob(f"*/{arg}.jsonl"))
    if not hits:
        sys.exit(f"no session found for {arg!r}")
    return hits[0]


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("session")
    ap.add_argument("--compare", metavar="OTHER")
    ap.add_argument("--json", action="store_true")
    args = ap.parse_args()
    stats = compute(resolve(args.session))
    if args.compare:
        deltas = compare(stats, compute(resolve(args.compare)))
        for key, (va, vb, pct) in deltas.items():
            print(f"{key:24} {va:>14,} -> {vb:>14,}  {pct:+.1f}%")
    elif args.json:
        print(json.dumps(stats, indent=2))
    else:
        for key, val in stats.items():
            print(f"{key:24} {val}")


if __name__ == "__main__":
    main()
