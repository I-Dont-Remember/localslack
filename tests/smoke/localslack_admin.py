"""Client for LocalSlack's testing API (`/_localslack/*`), see docs/TESTING_API.md.

Real HTTP semantics: 2xx bodies are the resource (204 -> None); status >= 400 raises
`AdminError` with the `{"error":{"code","message","details"?}}` fields. Lists return
`{"items","next_since","generation"}`; `apps()`/`faults()`/... return just the items.
One method per endpoint. Long-poll waits (`wait_calls`, `wait_deliveries`) block server-side,
so tests never sleep. An unmet wait raises `AdminTimeout`.
"""

from __future__ import annotations

from typing import Any

import httpx

PREFIX = "/_localslack"
LONG_POLL_SLACK_S = 5.0  # HTTP timeout headroom over a server-side long poll


class AdminError(Exception):
    """A control endpoint answered with HTTP status >= 400."""

    def __init__(self, status: int, body: Any, path: str):
        self.status = status
        self.body = body
        self.path = path
        err = body.get("error") if isinstance(body, dict) else None
        err = err if isinstance(err, dict) else {}
        self.code: str | None = err.get("code")
        self.message: str | None = err.get("message")
        self.details: list[dict[str, Any]] = err.get("details") or []
        detail = f"{self.code}: {self.message}" if err else repr(body)
        super().__init__(f"{path}: HTTP {status} {detail}")


class AdminTimeout(AssertionError):
    """A long-poll wait ended before `min` items arrived."""


def _clean(params: dict[str, Any]) -> dict[str, Any]:
    """Drop Nones and render bools the way the server parses them."""
    out: dict[str, Any] = {}
    for key, value in params.items():
        if value is None:
            continue
        out[key] = str(value).lower() if isinstance(value, bool) else value
    return out


class Admin:
    """Thin, stateless wrapper over the testing API."""

    def __init__(self, base_url: str, *, transport: httpx.BaseTransport | None = None):
        self.base_url = base_url.rstrip("/")
        self._http = httpx.Client(base_url=self.base_url, transport=transport, timeout=10.0)

    def close(self) -> None:
        self._http.close()

    def _request(
        self,
        verb: str,
        path: str,
        *,
        params: dict[str, Any] | None = None,
        json: Any = None,
        content: str | None = None,
        content_type: str | None = None,
        timeout: float | None = None,
    ) -> Any:
        headers = {"Content-Type": content_type} if content_type else None
        resp = self._http.request(
            verb,
            PREFIX + path,
            params=_clean(params or {}),
            json=json,
            content=content,
            headers=headers,
            timeout=timeout if timeout is not None else httpx.USE_CLIENT_DEFAULT,
        )
        if not resp.content:
            body = None
        else:
            try:
                body = resp.json()
            except ValueError:
                body = resp.text
        if resp.status_code >= 400:
            raise AdminError(resp.status_code, body, PREFIX + path)
        return body

    def _poll_timeout(self, timeout_ms: int) -> float:
        return timeout_ms / 1000 + LONG_POLL_SLACK_S

    # --- health / state ---------------------------------------------------------------------

    def health(self) -> dict[str, Any]:
        return self._request("GET", "/health")

    def state(self) -> dict[str, Any]:
        return self._request("GET", "/state")

    # --- channel members --------------------------------------------------------------------

    def add_member(self, channel_id: str, user_id: str) -> None:
        """Put a user (human or app bot) in any conversation; idempotent, emits no events."""
        self._request("PUT", f"/channels/{channel_id}/members/{user_id}")

    def remove_member(self, channel_id: str, user_id: str) -> None:
        """Take a user out of any conversation; idempotent, emits no events."""
        self._request("DELETE", f"/channels/{channel_id}/members/{user_id}")

    # --- apps -------------------------------------------------------------------------------

    def register_app(
        self,
        manifest: dict[str, Any] | str,
        *,
        signing_secret: str | None = None,
        bot_token: str | None = None,
        app_token: str | None = None,
        app_id: str | None = None,
        verify: bool | None = None,
        deliver_to: str | None = None,
    ) -> dict[str, Any]:
        """Register an app from a manifest (dict = JSON, str = YAML); returns the response."""
        extras = {
            "signing_secret": signing_secret,
            "bot_token": bot_token,
            "app_token": app_token,
            "app_id": app_id,
            "verify": verify,
            "deliver_to": deliver_to,
        }
        extras = {k: v for k, v in extras.items() if v is not None}
        if extras:
            return self._request("POST", "/apps", json={"manifest": manifest, **extras})
        if isinstance(manifest, str):
            return self._request("POST", "/apps", content=manifest, content_type="application/yaml")
        return self._request("POST", "/apps", json=manifest)

    def apps(self) -> list[dict[str, Any]]:
        return self._request("GET", "/apps")["items"]

    def app(self, app_id: str) -> dict[str, Any]:
        return self._request("GET", f"/apps/{app_id}")

    def patch_app(self, app_id: str, **fields: Any) -> dict[str, Any]:
        """Update an app (currently `deliver_to`); returns the app."""
        return self._request("PATCH", f"/apps/{app_id}", json=fields)

    def set_url(self, app_id: str, url: str) -> dict[str, Any]:
        """Point an app's event delivery at `url` (its `deliver_to`)."""
        return self.patch_app(app_id, deliver_to=url)

    def delete_app(self, app_id: str) -> None:
        self._request("DELETE", f"/apps/{app_id}")

    def verify_app(self, app_id: str) -> dict[str, Any]:
        return self._request("POST", f"/apps/{app_id}/verify")

    # --- events / deliveries ----------------------------------------------------------------

    def deliveries(
        self,
        *,
        event_type: str | None = None,
        state: str | None = None,
        app_id: str | None = None,
        since: int | None = None,
        min: int | None = None,
        timeout_ms: int | None = None,
    ) -> dict[str, Any]:
        params = {
            "event_type": event_type,
            "state": state,
            "app_id": app_id,
            "since": since,
            "min": min,
            "timeout_ms": timeout_ms,
        }
        timeout = self._poll_timeout(timeout_ms) if timeout_ms else None
        return self._request("GET", "/deliveries", params=params, timeout=timeout)

    def wait_deliveries(self, min: int, timeout_ms: int = 5000, **filters: Any) -> list[dict]:
        """Block until `min` deliveries match; raise AdminTimeout if they do not arrive."""
        body = self.deliveries(min=min, timeout_ms=timeout_ms, **filters)
        got = body["items"]
        if len(got) < min:
            raise AdminTimeout(f"wanted {min} deliveries within {timeout_ms}ms, got {len(got)}")
        return got

    def delivery(self, delivery_id: str) -> dict[str, Any]:
        return self._request("GET", f"/deliveries/{delivery_id}")

    def delete_deliveries(self) -> None:
        self._request("DELETE", "/deliveries")

    def inject_event(
        self,
        event: dict[str, Any],
        *,
        app_id: str | None = None,
        signature: str | None = None,
        envelope: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        body = {"event": event, "app_id": app_id, "signature": signature, "envelope": envelope}
        return self._request(
            "POST", "/events", json={k: v for k, v in body.items() if v is not None}
        )

    def redeliver(
        self,
        event_id: str,
        *,
        app_id: str | None = None,
        retry_num: int | None = None,
        retry_reason: str | None = None,
        signature: str | None = None,
    ) -> dict[str, Any]:
        """Resend by event id. Event ids are per app delivery, so `app_id` is optional."""
        body = {
            "app_id": app_id,
            "retry_num": retry_num,
            "retry_reason": retry_reason,
            "signature": signature,
        }
        body = {k: v for k, v in body.items() if v is not None}
        return self._request("POST", f"/events/{event_id}/redeliver", json=body)

    def redeliver_delivery(
        self,
        delivery_id: str,
        *,
        retry_num: int | None = None,
        retry_reason: str | None = None,
        signature: str | None = None,
    ) -> dict[str, Any]:
        body = {"retry_num": retry_num, "retry_reason": retry_reason, "signature": signature}
        body = {k: v for k, v in body.items() if v is not None}
        return self._request("POST", f"/deliveries/{delivery_id}/redeliver", json=body)

    def delivery_config(self, app_id: str | None = None) -> dict[str, Any]:
        """Global config, or an app's effective config plus its sparse `override`."""
        return self._request("GET", self._config_path(app_id))

    def put_delivery_config(self, app_id: str | None = None, **config: Any) -> dict[str, Any]:
        """Replace: fields not given go back to their defaults (globally) or inherit (per app)."""
        return self._request("PUT", self._config_path(app_id), json=config)

    def patch_delivery_config(self, app_id: str | None = None, **config: Any) -> dict[str, Any]:
        """Merge the given fields into the current config (globally) or the app's override."""
        return self._request("PATCH", self._config_path(app_id), json=config)

    @staticmethod
    def _config_path(app_id: str | None) -> str:
        return f"/apps/{app_id}/delivery-config" if app_id else "/delivery-config"

    # --- socket mode ------------------------------------------------------------------------

    def socket_connections(self) -> list[dict[str, Any]]:
        return self._request("GET", "/socket/connections")["items"]

    def socket_disconnect(
        self,
        *,
        connection_id: str | None = None,
        reason: str = "refresh_requested",
        drop: bool = False,
    ) -> None:
        body = {"connection_id": connection_id, "reason": reason, "drop": drop}
        self._request(
            "POST", "/socket/disconnect", json={k: v for k, v in body.items() if v is not None}
        )

    # --- call log ---------------------------------------------------------------------------

    def head_seq(self) -> int:
        """Seq of the newest recorded call (0 when empty).

        `calls(limit=1)` is the oldest page, so its `next_since` is not the head.
        """
        since = 0
        while page := self.calls(since=since, limit=1000)["items"]:
            since = page[-1]["seq"]
        return since

    def calls(
        self,
        *,
        method: str | None = None,
        channel: str | None = None,
        app_id: str | None = None,
        ok: bool | None = None,
        error: str | None = None,
        since: int | None = None,
        limit: int | None = None,
        min: int | None = None,
        timeout_ms: int | None = None,
    ) -> dict[str, Any]:
        params = {
            "method": method,
            "channel": channel,
            "app_id": app_id,
            "ok": ok,
            "error": error,
            "since": since,
            "limit": limit,
            "min": min,
            "timeout_ms": timeout_ms,
        }
        timeout = self._poll_timeout(timeout_ms) if timeout_ms else None
        return self._request("GET", "/calls", params=params, timeout=timeout)

    def wait_calls(self, min: int, timeout_ms: int = 5000, **filters: Any) -> list[dict]:
        """Block until `min` calls match; raise AdminTimeout if they do not arrive."""
        got = self.calls(min=min, timeout_ms=timeout_ms, **filters)["items"]
        if len(got) < min:
            raise AdminTimeout(f"wanted {min} calls within {timeout_ms}ms, got {len(got)}")
        return got

    def delete_calls(self) -> None:
        self._request("DELETE", "/calls")

    # --- reset ------------------------------------------------------------------------------

    def reset(
        self,
        all: bool = False,
        *,
        seed: str | None = None,
        scenario: dict[str, Any] | str | None = None,
        generate: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Partial reset (keeps apps) by default; `all=True` also removes apps."""
        body = {"seed": seed, "scenario": scenario, "generate": generate}
        return self._request(
            "POST",
            "/reset",
            params={"all": True} if all else None,
            json={k: v for k, v in body.items() if v is not None},
        )

    # --- faults -----------------------------------------------------------------------------

    def add_fault(
        self,
        method: str,
        action: dict[str, Any],
        *,
        match: dict[str, Any] | None = None,
        nth: int | None = None,
        every: int | None = None,
        times: int | None = None,
        once: bool | None = None,
    ) -> dict[str, Any]:
        """Add a fault rule; returns the created fault (with its `id`)."""
        body = {
            "method": method,
            "match": match,
            "nth": nth,
            "every": every,
            "times": times,
            "once": once,
            "action": action,
        }
        body = {k: v for k, v in body.items() if v is not None}
        return self._request("POST", "/faults", json=body)

    def faults(self) -> list[dict[str, Any]]:
        return self._request("GET", "/faults")["items"]

    def delete_fault(self, fault_id: str) -> None:
        self._request("DELETE", f"/faults/{fault_id}")

    def delete_faults(self) -> None:
        self._request("DELETE", "/faults")

    # --- snapshots / diff -------------------------------------------------------------------

    def create_snapshot(self, name: str) -> dict[str, Any]:
        return self._request("POST", "/snapshots", json={"name": name})

    def snapshots(self) -> list[dict[str, Any]]:
        return self._request("GET", "/snapshots")["items"]

    def snapshot(self, name: str) -> dict[str, Any]:
        """The snapshot as a Fixture (also usable as a scenario)."""
        return self._request("GET", f"/snapshots/{name}")

    def restore_snapshot(self, name: str) -> dict[str, Any]:
        return self._request("POST", f"/snapshots/{name}/restore")

    def diff(self, from_: str, to: str = "current") -> dict[str, Any]:
        return self._request("GET", "/diff", params={"from": from_, "to": to})
