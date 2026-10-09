"""Record a real Slack API response as a sanitized conformance fixture.

Usage (needs a token for a throwaway workspace; never commit tokens):
    SLACK_TOKEN=xoxb-... uv run --project tests/smoke python tests/conformance/record.py \
        auth.test [key=value ...]

Also set SLACK_WORKSPACE_TYPE (free|pro|enterprise|sandbox); it is recorded as provenance.
Writes tests/conformance/fixtures/<method>.json with state "known_gap" and source "recorded".
Review the output, then flip state to "passing" once LocalSlack matches.
"""

import datetime
import json
import os
import re
import sys
from pathlib import Path

import httpx

FIXTURES = Path(__file__).parent / "fixtures"
TS = re.compile(r"^\d{10}\.\d{6}$")
ID = re.compile(r"^([A-Z])[A-Z0-9]{8,}$")
# Keys whose values are free-form text/urls: keep shape only.
FREEFORM = {
    "url",
    "team",
    "user",
    "text",
    "name",
    "title",
    "real_name",
    "email",
    "bot_id",
}


def sanitize(value, key=""):
    """Replace volatile/identifying values with conformance matchers."""
    if isinstance(value, dict):
        return {k: sanitize(v, k) for k, v in value.items()}
    if isinstance(value, list):
        return [sanitize(v, key) for v in value]
    if isinstance(value, str):
        if TS.match(value):
            return "<ts>"
        if m := ID.match(value):
            return f"<id:{m.group(1)}>"
        if key in FREEFORM and value:
            return "<string>"
    return value


def main() -> int:
    workspace_type = os.environ.get("SLACK_WORKSPACE_TYPE", "")
    if (
        len(sys.argv) < 2
        or not (token := os.environ.get("SLACK_TOKEN"))
        or not workspace_type
    ):
        print(__doc__, file=sys.stderr)
        return 2
    method = sys.argv[1]
    params = dict(arg.split("=", 1) for arg in sys.argv[2:])
    resp = httpx.post(
        f"https://slack.com/api/{method}",
        headers={"Authorization": f"Bearer {token}"},
        data=params,
    )
    fixture = {
        "state": "known_gap",
        "note": f"recorded from real Slack: {method}",
        "source": "recorded",
        "recorded": {
            "date": datetime.date.today().isoformat(),
            "tool": "record.py",
            "workspace_type": workspace_type,
            "token_type": "user" if token.startswith("xoxp-") else "bot",
            "scopes": [
                scope.strip()
                for scope in resp.headers.get("x-oauth-scopes", "").split(",")
                if scope.strip()
            ],
        },
        "request": {
            "method": "POST",
            "path": f"/api/{method}",
            "token": "xoxb-test",
            "params": params,
        },
        "response": {"status": resp.status_code, "body": sanitize(resp.json())},
    }
    out = FIXTURES / f"{method.replace('.', '_')}.json"
    out.write_text(json.dumps(fixture, indent=2) + "\n")
    print(f"wrote {out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
