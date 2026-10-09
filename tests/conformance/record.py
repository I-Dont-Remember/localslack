"""Record real Slack responses as sanitized conformance fixtures.

Needs a token for a THROWAWAY workspace (never commit tokens or real data):

    export SLACK_TOKEN=xoxb-...            # token for the throwaway workspace
    export SLACK_WORKSPACE_TYPE=free       # free | pro | enterprise | sandbox (stored as provenance)

    # Upgrade an existing fixture: replays its setup + request against real Slack.
    uv run --project tests/smoke python tests/conformance/record.py \\
        --from tests/conformance/fixtures/conversations_replies.json

    # Ad hoc: record one method with params.
    uv run --project tests/smoke python tests/conformance/record.py conversations.list limit=2

Output is `<name>__recorded.json` next to the fixtures (an existing file is never overwritten);
`--replace` with `--from` overwrites the source fixture instead. The new fixture is a
`known_gap`: run `make conformance-update-gaps` to record how LocalSlack differs, then run
`make redact-check` and review the diff by hand before committing.

Recording creates real objects (e.g. channels) in the workspace; a random suffix is added to
`name` params on the wire so reruns do not collide, while the saved fixture keeps the original.
"""

import argparse
import datetime
import json
import os
import re
import sys
import uuid
from pathlib import Path

import httpx

FIXTURES = Path(__file__).parent / "fixtures"
TS = re.compile(r"^\d{10}\.\d{6}$")
ID = re.compile(r"^([A-Z])[A-Z0-9]{8,}$")
EMAIL = re.compile(r"[\w.+-]+@[\w-]+\.[\w.-]+")
URL = re.compile(r"^https?://", re.IGNORECASE)
LONG_TEXT = 60
# Keys whose values are free-form text: keep shape only.
FREEFORM = {
    "url", "team", "user", "text", "name", "title", "real_name", "email", "bot_id",
    "value", "display_name", "first_name", "last_name", "status_text", "phone",
}  # fmt: skip
# Keys holding epoch numbers that change every run.
TIME_KEYS = {
    "created",
    "updated",
    "latest",
    "date_create",
    "last_read",
    "unread_count_display",
}
# Response headers worth keeping as evidence of real behavior.
KEEP_HEADERS = ("x-oauth-scopes", "x-accepted-oauth-scopes", "retry-after")
WIRE_UNIQUE_PARAMS = {"name"}


def sanitize(value, key=""):
    """Replace volatile or identifying values with conformance matchers."""
    if isinstance(value, dict):
        return {k: sanitize(v, k) for k, v in value.items()}
    if isinstance(value, list):
        return [sanitize(v, key) for v in value]
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)) and key in TIME_KEYS:
        return "<any>"
    if isinstance(value, str) and value:
        if TS.match(value):
            return "<ts>"
        if m := ID.match(value):
            return f"<id:{m.group(1)}>"
        if (
            key in FREEFORM
            or URL.match(value)
            or EMAIL.search(value)
            or (len(value) > LONG_TEXT and " " in value)
        ):
            return "<string>"
    return value


def dig(body, path):
    """Read a dotted path out of a decoded JSON body (as a string)."""
    cur = body
    for part in path.split("."):
        if not isinstance(cur, dict) or part not in cur:
            raise KeyError(f"capture path {path!r} not found in {body}")
        cur = cur[part]
    return str(cur)


def substitute(text: str, caps: dict[str, str]) -> str:
    for name, value in caps.items():
        text = text.replace("{{" + name + "}}", value)
    return text


def call(client: httpx.Client, base: str, token: str, step: dict, caps, unique: str):
    params = {}
    for k, v in (step.get("params") or {}).items():
        v = substitute(v, caps)
        params[k] = f"{v}-{unique}" if k in WIRE_UNIQUE_PARAMS and "{{" not in v else v
    return client.post(
        base + substitute(step["path"], caps),
        headers={"Authorization": f"Bearer {token}"},
        data=params,
    )


def keep_headers(resp: httpx.Response) -> dict[str, str]:
    return {h: resp.headers[h] for h in KEEP_HEADERS if h in resp.headers}


def build_fixture(name, steps, main_req, resp, workspace_type, token):
    headers = keep_headers(resp)
    scopes = [
        s.strip() for s in headers.get("x-oauth-scopes", "").split(",") if s.strip()
    ]
    body = resp.json()
    recorded = {
        "date": datetime.date.today().isoformat(),
        "tool": "record.py",
        "workspace_type": workspace_type,
        "token_type": "user" if token.startswith("xoxp-") else "bot",
        "scopes": scopes,
    }
    if headers:
        recorded["headers"] = headers
    fixture = {
        "state": "known_gap",
        "note": f"recorded from real Slack: {name}",
        "source": "recorded",
        "recorded": recorded,
        "request": {**main_req, "token": "xoxb-test"},
        "response": {"status": resp.status_code, "body": sanitize(body)},
    }
    if steps:
        fixture["setup"] = [{**s, "token": "xoxb-test"} for s in steps]
    return fixture


def parse_args(argv):
    p = argparse.ArgumentParser(
        description="Record sanitized Slack responses as fixtures."
    )
    p.add_argument("method", nargs="?", help="Slack method for an ad hoc recording")
    p.add_argument("params", nargs="*", help="key=value request params")
    p.add_argument(
        "--from", dest="src", help="existing fixture to replay against real Slack"
    )
    p.add_argument("--base-url", default="https://slack.com")
    p.add_argument(
        "--workspace-type", help="free|pro|enterprise|sandbox (or SLACK_WORKSPACE_TYPE)"
    )
    p.add_argument("--out-dir", default=str(FIXTURES))
    p.add_argument(
        "--replace", action="store_true", help="with --from: overwrite the source file"
    )
    return p.parse_args(argv)


def main(argv=None, env=None) -> int:
    env = os.environ if env is None else env
    args = parse_args(sys.argv[1:] if argv is None else argv)
    token = env.get("SLACK_TOKEN")
    workspace_type = args.workspace_type or env.get("SLACK_WORKSPACE_TYPE")
    if not token or not workspace_type or not (args.src or args.method):
        print(__doc__, file=sys.stderr)
        return 2

    if args.src:
        src = Path(args.src)
        fixture = json.loads(src.read_text())
        steps, main_req = fixture.get("setup", []), fixture["request"]
        name = src.stem
        dest = src if args.replace else Path(args.out_dir) / f"{name}__recorded.json"
    else:
        steps = []
        params = dict(a.split("=", 1) for a in args.params)
        main_req = {"method": "POST", "path": f"/api/{args.method}", "params": params}
        name = args.method.replace(".", "_")
        dest = Path(args.out_dir) / f"{name}.json"
    if dest.exists() and not (args.replace and args.src):
        print(
            f"refusing to overwrite {dest}; delete it or pick another --out-dir",
            file=sys.stderr,
        )
        return 1

    unique, caps = uuid.uuid4().hex[:8], {}
    with httpx.Client(timeout=30) as client:
        for step in steps:
            resp = call(client, args.base_url, token, step, caps, unique)
            body = resp.json()
            for cap_name, path in (step.get("capture") or {}).items():
                caps[cap_name] = dig(body, path)
        resp = call(client, args.base_url, token, main_req, caps, unique)
    if resp.status_code == 429:
        print(
            f"rate limited by Slack; retry after {resp.headers.get('retry-after')}s",
            file=sys.stderr,
        )
        return 1

    out = build_fixture(name, steps, main_req, resp, workspace_type, token)
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text(json.dumps(out, indent=2) + "\n")
    print(f"wrote {dest}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
