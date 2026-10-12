"""A tiny HTTP server that feeds a real Bolt for Python app, for Events API smoke tests.

Listens on a free loopback port in a background thread, hands each POST to
`app.dispatch(BoltRequest)` and relays Bolt's response. It records the headers of every request
so tests can assert on `X-Slack-Retry-Num` and friends.
"""

from __future__ import annotations

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

from slack_bolt import App, BoltRequest


class BoltServer:
    def __init__(self, path: str = "/slack/events"):
        self.path = path
        self.app: App | None = None
        self.requests: list[dict[str, Any]] = []
        outer = self

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):  # noqa: N802 (http.server API)
                body = self.rfile.read(int(self.headers.get("Content-Length", 0))).decode()
                headers = {k.lower(): [v] for k, v in self.headers.items()}
                outer.requests.append({"headers": dict(self.headers), "body": body})
                if outer.app is None and '"url_verification"' in body:
                    # Registration handshake arrives before the App can exist: echo like Bolt does.
                    payload = json.dumps({"challenge": json.loads(body)["challenge"]}).encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(payload)))
                    self.end_headers()
                    self.wfile.write(payload)
                    return
                if outer.app is None or self.path != outer.path:
                    self.send_response(404)
                    self.end_headers()
                    return
                resp = outer.app.dispatch(BoltRequest(body=body, headers=headers))
                self.send_response(resp.status)
                for key, values in (resp.headers or {}).items():
                    for value in values:
                        self.send_header(key, value)
                payload = (resp.body or "").encode()
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)

            def log_message(self, *args):  # keep test output quiet
                pass

        self._httpd = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self._thread = threading.Thread(target=self._httpd.serve_forever, daemon=True)
        self._thread.start()

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self._httpd.server_port}{self.path}"

    def close(self) -> None:
        self._httpd.shutdown()
        self._httpd.server_close()
