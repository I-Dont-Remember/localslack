"""Tests for scripts/trace_stats.py against synthetic session JSONL."""

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "scripts"))

import trace_stats


def _assistant(ts, tool=None, command=None, usage=None, api_error=None):
    content = []
    if tool:
        inp = {"command": command} if command is not None else {}
        content.append(
            {"type": "tool_use", "id": f"id-{ts}", "name": tool, "input": inp}
        )
    msg = {"type": "assistant", "timestamp": ts, "message": {"content": content}}
    if usage:
        msg["message"]["usage"] = usage
    if api_error:
        msg["isApiErrorMessage"] = True
        msg["message"]["content"] = [{"type": "text", "text": api_error}]
    return msg


def _result(ts, text, is_error=False):
    block = {
        "type": "tool_result",
        "tool_use_id": "x",
        "content": text,
        "is_error": is_error,
    }
    return {"type": "user", "timestamp": ts, "message": {"content": [block]}}


def _write(path: Path, rows):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("\n".join(json.dumps(r) for r in rows) + "\n")


def _usage(cache_read, out=10, cache_write=5):
    return {
        "input_tokens": 1,
        "output_tokens": out,
        "cache_read_input_tokens": cache_read,
        "cache_creation_input_tokens": cache_write,
    }


def test_compute_counts_mechanism_metrics(tmp_path):
    main = tmp_path / "s1.jsonl"
    _write(
        main,
        [
            _assistant(
                "2026-10-12T10:00:00Z", "Bash", "sed -i 's/a/b/' x.go", _usage(100)
            ),
            _result(
                "2026-10-12T10:00:01Z", "too complex to verify that it stays inside"
            ),
            _assistant(
                "2026-10-12T10:00:02Z",
                "Bash",
                "python3 - <<'E'\nprint(1)\nE",
                _usage(300),
            ),
            _assistant("2026-10-12T10:00:03Z", "Bash", "make check", _usage(200)),
            _assistant("2026-10-12T10:00:04Z", "Bash", "make test", _usage(200)),
            _assistant("2026-10-12T10:00:05Z", "Edit", None, _usage(200)),
            _assistant(
                "2026-10-12T10:00:06Z",
                api_error="You've hit your session limit · resets 4:30pm",
            ),
            _assistant("2026-10-12T14:00:06Z", "Bash", "git status", _usage(100)),
        ],
    )
    got = trace_stats.compute(main)
    want = {
        "turns": 7,
        "tool_calls": {"Bash": 5, "Edit": 1},
        "bash_edit_calls": 2,
        "guard_refusals": 1,
        "session_limit_hits": 1,
        "full_suite_runs": {"make check": 1, "make test": 1, "make verify": 0},
        "output_tokens": 60,
        "cache_read_tokens": 1100,
        "cache_write_tokens": 30,
        "peak_context": 300,
        "idle_hours": 4.0,
    }
    assert {k: got[k] for k in want} == want


def test_compute_includes_subagents_and_concurrency(tmp_path):
    main = tmp_path / "s1.jsonl"
    _write(main, [_assistant("2026-10-12T10:00:00Z", "Agent", None, _usage(10))])
    sub = tmp_path / "s1" / "subagents"
    for i, (start, end) in enumerate(
        [("10:00:00", "10:10:00"), ("10:01:00", "10:09:00"), ("10:20:00", "10:30:00")]
    ):
        _write(
            sub / f"agent-{i}.jsonl",
            [
                _assistant(f"2026-10-12T{start}Z", "Bash", "ls", _usage(50)),
                _assistant(f"2026-10-12T{end}Z", "Bash", "ls", _usage(50)),
            ],
        )
    got = trace_stats.compute(main)
    assert got["max_concurrency"] == 2
    assert got["cache_read_tokens"] == 10 + 6 * 50
    assert got["subagents"] == 3


def test_tool_result_bytes_percentiles(tmp_path):
    main = tmp_path / "s1.jsonl"
    rows = []
    for i, n in enumerate([10, 20, 30, 40, 1000]):
        rows.append(_result(f"2026-10-12T10:00:{i:02d}Z", "x" * n))
    _write(main, rows)
    got = trace_stats.compute(main)
    assert got["result_bytes_p50"] == 30
    assert got["result_bytes_p95"] == 1000


def test_compare_prints_deltas():
    a = {"cache_read_tokens": 1000, "guard_refusals": 10}
    b = {"cache_read_tokens": 500, "guard_refusals": 0}
    got = trace_stats.compare(a, b)
    assert got["cache_read_tokens"] == (1000, 500, -50.0)
    assert got["guard_refusals"] == (10, 0, -100.0)
