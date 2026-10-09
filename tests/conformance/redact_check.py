"""Fail if a conformance fixture could leak workspace data (run by `make redact-check`).

Recorded fixtures come from real workspaces, sometimes via community contributors, so every
fixture is scanned for tokens, emails, workspace URLs, literal Slack IDs and long free text.
Usage: python tests/conformance/redact_check.py [fixtures-dir ...]
"""

import json
import re
import sys
from pathlib import Path

TOKEN = re.compile(r"\b(?:xox[a-z]|xapp)-[0-9A-Za-z-]{10,}")
EMAIL = re.compile(r"[\w.+-]+@[\w-]+\.[\w.-]+")
URL = re.compile(r"https?://([\w.-]+)", re.IGNORECASE)
ID = re.compile(r"^[TUCDGWB][A-Z0-9]{8,}$")
PLACEHOLDER_ID = re.compile(r"^[A-Z]0{8,}$")  # C00000000 etc: obviously fake
OK_HOSTS = {"slack.com", "www.slack.com", "api.slack.com", "docs.slack.dev"}
WORKSPACE_HOST = re.compile(
    r"(\.slack\.com|slack-edge\.com|slack-files\.com)$", re.IGNORECASE
)
LONG_TEXT = 80
# Metadata keys that legitimately hold prose or citations.
PROSE_KEYS = {"note", "notes", "ref", "gap_diffs"}


def find_problems(node, path="$") -> list[str]:
    """Return human-readable leak candidates found anywhere in a JSON-like value."""
    problems: list[str] = []
    if isinstance(node, dict):
        for key, value in node.items():
            problems += find_problems(value, f"{path}.{key}")
    elif isinstance(node, list):
        for i, value in enumerate(node):
            problems += find_problems(value, f"{path}[{i}]")
    elif isinstance(node, str):
        problems += _string_problems(node, path)
    return problems


def _string_problems(value: str, path: str) -> list[str]:
    out = []
    if TOKEN.search(value):
        out.append(f"{path}: looks like a Slack token")
    if EMAIL.search(value):
        out.append(f"{path}: contains an email address")
    for host in URL.findall(value):
        if host.lower() not in OK_HOSTS and WORKSPACE_HOST.search(host):
            out.append(f"{path}: contains a workspace slack url ({host})")
    if ID.match(value) and not PLACEHOLDER_ID.match(value):
        out.append(f"{path}: literal Slack id {value!r}; use an <id:X> matcher")
    last = path.rsplit(".", 1)[-1].split("[")[0]
    if len(value) > LONG_TEXT and not value.startswith("<") and last not in PROSE_KEYS:
        out.append(
            f"{path}: long free text ({len(value)} chars); replace with <string>"
        )
    return out


def scan_dir(directory: Path) -> dict[str, list[str]]:
    """Map fixture file name -> problems, omitting clean files."""
    found: dict[str, list[str]] = {}
    for file in sorted(directory.glob("*.json")):
        problems = find_problems(json.loads(file.read_text()))
        if problems:
            found[file.name] = problems
    return found


def main(argv: list[str]) -> int:
    dirs = [Path(a) for a in argv] or [Path(__file__).parent / "fixtures"]
    bad = {name: p for d in dirs for name, p in scan_dir(d).items()}
    for name, problems in bad.items():
        print(f"{name}:", file=sys.stderr)
        for problem in problems:
            print(f"  {problem}", file=sys.stderr)
    if bad:
        print(
            "redact-check failed: fix the fixture(s) before committing", file=sys.stderr
        )
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
