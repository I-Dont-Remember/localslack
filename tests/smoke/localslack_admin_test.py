"""Unit tests for the Admin helper against httpx.MockTransport (no server needed)."""

import json

import httpx
import pytest

from localslack_admin import Admin, AdminError, AdminTimeout


def make_admin(reply=None, status=200):
    """An Admin whose requests are captured in `seen`; every call answers `reply`."""
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(status, json=reply if reply is not None else {})

    return Admin("http://x", transport=httpx.MockTransport(handler)), seen


def body(request: httpx.Request):
    return json.loads(request.content) if request.content else None


CASES = [
    # (name, call, verb, path, query, json body, reply)
    ("health", lambda a: a.health(), "GET", "/_localslack/health", {}, None, None),
    (
        "calls filters",
        lambda a: a.calls(method="chat.postMessage", ok=False, since=3, limit=5),
        "GET",
        "/_localslack/calls",
        {"method": "chat.postMessage", "ok": "false", "since": "3", "limit": "5"},
        None,
        {"items": [], "next_since": 0, "generation": 1},
    ),
    ("delete calls", lambda a: a.delete_calls(), "DELETE", "/_localslack/calls", {}, None, None),
    (
        "add fault",
        lambda a: a.add_fault("chat.postMessage", {"status": 429}, nth=3, match={"channel": "C1"}),
        "POST",
        "/_localslack/faults",
        {},
        {
            "method": "chat.postMessage",
            "nth": 3,
            "match": {"channel": "C1"},
            "action": {"status": 429},
        },
        {"id": "f_1"},
    ),
    (
        "delete fault",
        lambda a: a.delete_fault("f_1"),
        "DELETE",
        "/_localslack/faults/f_1",
        {},
        None,
        None,
    ),
    ("delete faults", lambda a: a.delete_faults(), "DELETE", "/_localslack/faults", {}, None, None),
    (
        "register raw manifest",
        lambda a: a.register_app({"display_information": {"name": "d"}}),
        "POST",
        "/_localslack/apps",
        {},
        {"display_information": {"name": "d"}},
        None,
    ),
    (
        "register with credentials",
        lambda a: a.register_app({"k": 1}, signing_secret="s", verify=True),
        "POST",
        "/_localslack/apps",
        {},
        {"manifest": {"k": 1}, "signing_secret": "s", "verify": True},
        None,
    ),
    (
        "verify app",
        lambda a: a.verify_app("A1"),
        "POST",
        "/_localslack/apps/A1/verify",
        {},
        None,
        None,
    ),
    ("delete app", lambda a: a.delete_app("A1"), "DELETE", "/_localslack/apps/A1", {}, None, None),
    (
        "deliveries filters",
        lambda a: a.deliveries(event_type="app_mention", state="failed", app_id="A1"),
        "GET",
        "/_localslack/deliveries",
        {"event_type": "app_mention", "state": "failed", "app_id": "A1"},
        None,
        {"items": [], "next_since": 0, "generation": 1},
    ),
    (
        "delete deliveries",
        lambda a: a.delete_deliveries(),
        "DELETE",
        "/_localslack/deliveries",
        {},
        None,
        None,
    ),
    (
        "inject event",
        lambda a: a.inject_event({"type": "message"}, app_id="A1", signature="invalid"),
        "POST",
        "/_localslack/events",
        {},
        {"event": {"type": "message"}, "app_id": "A1", "signature": "invalid"},
        None,
    ),
    (
        "redeliver",
        lambda a: a.redeliver("Ev1", retry_num=1, retry_reason="http_timeout"),
        "POST",
        "/_localslack/events/Ev1/redeliver",
        {},
        {"retry_num": 1, "retry_reason": "http_timeout"},
        None,
    ),
    (
        "redeliver by delivery id",
        lambda a: a.redeliver_delivery("D1", retry_num=2),
        "POST",
        "/_localslack/deliveries/D1/redeliver",
        {},
        {"retry_num": 2},
        None,
    ),
    (
        "redeliver with app id",
        lambda a: a.redeliver("Ev1", app_id="A1"),
        "POST",
        "/_localslack/events/Ev1/redeliver",
        {},
        {"app_id": "A1"},
        None,
    ),
    (
        "get delivery config",
        lambda a: a.delivery_config(),
        "GET",
        "/_localslack/delivery-config",
        {},
        None,
        None,
    ),
    (
        "put delivery config",
        lambda a: a.put_delivery_config(signature="stale", paused=True),
        "PUT",
        "/_localslack/delivery-config",
        {},
        {"signature": "stale", "paused": True},
        None,
    ),
    (
        "patch delivery config",
        lambda a: a.patch_delivery_config(paused=False),
        "PATCH",
        "/_localslack/delivery-config",
        {},
        {"paused": False},
        None,
    ),
    (
        "patch per-app delivery config",
        lambda a: a.patch_delivery_config("A1", signature="invalid"),
        "PATCH",
        "/_localslack/apps/A1/delivery-config",
        {},
        {"signature": "invalid"},
        None,
    ),
    (
        "put per-app delivery config",
        lambda a: a.put_delivery_config("A1", timeout_ms=100),
        "PUT",
        "/_localslack/apps/A1/delivery-config",
        {},
        {"timeout_ms": 100},
        None,
    ),
    (
        "socket connections",
        lambda a: a.socket_connections(),
        "GET",
        "/_localslack/socket/connections",
        {},
        None,
        {"items": []},
    ),
    (
        "socket disconnect",
        lambda a: a.socket_disconnect(connection_id="c1", drop=True),
        "POST",
        "/_localslack/socket/disconnect",
        {},
        {"connection_id": "c1", "reason": "refresh_requested", "drop": True},
        None,
    ),
    (
        "reset",
        lambda a: a.reset(seed="s"),
        "POST",
        "/_localslack/reset",
        {},
        {"seed": "s"},
        None,
    ),
    (
        "reset all",
        lambda a: a.reset(all=True),
        "POST",
        "/_localslack/reset",
        {"all": "true"},
        {},
        None,
    ),
    (
        "register with deliver_to",
        lambda a: a.register_app({"k": 1}, deliver_to="http://localhost:3000"),
        "POST",
        "/_localslack/apps",
        {},
        {"manifest": {"k": 1}, "deliver_to": "http://localhost:3000"},
        None,
    ),
    (
        "patch app",
        lambda a: a.patch_app("A1", deliver_to="http://localhost:4000"),
        "PATCH",
        "/_localslack/apps/A1",
        {},
        {"deliver_to": "http://localhost:4000"},
        None,
    ),
    (
        "set url",
        lambda a: a.set_url("A1", "http://localhost:4000"),
        "PATCH",
        "/_localslack/apps/A1",
        {},
        {"deliver_to": "http://localhost:4000"},
        None,
    ),
    ("list apps", lambda a: a.apps(), "GET", "/_localslack/apps", {}, None, {"items": []}),
    ("get app", lambda a: a.app("A1"), "GET", "/_localslack/apps/A1", {}, None, {"id": "A1"}),
    (
        "create snapshot",
        lambda a: a.create_snapshot("before"),
        "POST",
        "/_localslack/snapshots",
        {},
        {"name": "before"},
        None,
    ),
    (
        "restore snapshot",
        lambda a: a.restore_snapshot("before"),
        "POST",
        "/_localslack/snapshots/before/restore",
        {},
        None,
        None,
    ),
    (
        "diff",
        lambda a: a.diff("before"),
        "GET",
        "/_localslack/diff",
        {"from": "before", "to": "current"},
        None,
        {},
    ),
]


@pytest.mark.parametrize(
    ("call", "verb", "path", "query", "want_body", "reply"),
    [c[1:] for c in CASES],
    ids=[c[0] for c in CASES],
)
def test_request_shape(call, verb, path, query, want_body, reply):
    admin, seen = make_admin(reply)

    call(admin)

    (request,) = seen
    assert (request.method, request.url.path) == (verb, path)
    assert dict(request.url.params) == query
    assert body(request) == want_body


def test_register_yaml_manifest_is_sent_as_yaml():
    admin, seen = make_admin()

    admin.register_app("display_information:\n  name: d\n")

    assert seen[0].headers["content-type"] == "application/yaml"
    assert seen[0].content == b"display_information:\n  name: d\n"


def test_wait_calls_long_polls_and_returns_calls():
    admin, seen = make_admin({"items": [{"seq": 1}, {"seq": 2}], "next_since": 2, "generation": 1})

    got = admin.wait_calls(2, timeout_ms=1500, method="chat.postMessage")

    assert got == [{"seq": 1}, {"seq": 2}]
    assert dict(seen[0].url.params) == {
        "min": "2",
        "timeout_ms": "1500",
        "method": "chat.postMessage",
    }


def test_wait_calls_times_out_when_too_few():
    admin, _ = make_admin({"items": [], "next_since": 0, "generation": 1})

    with pytest.raises(AdminTimeout):
        admin.wait_calls(1, timeout_ms=10)


def test_wait_deliveries_returns_deliveries():
    admin, seen = make_admin({"items": [{"id": "d1"}], "next_since": 1, "generation": 1})

    assert admin.wait_deliveries(1, timeout_ms=200, app_id="A1") == [{"id": "d1"}]
    assert dict(seen[0].url.params) == {"min": "1", "timeout_ms": "200", "app_id": "A1"}


def test_add_fault_returns_the_created_fault():
    admin, _ = make_admin({"id": "f_9"})

    assert admin.add_fault("*", {"status": 500})["id"] == "f_9"


def test_calls_returns_list_envelope():
    envelope = {"items": [{"seq": 1}], "next_since": 1, "generation": 2}
    admin, _ = make_admin(envelope)

    assert admin.calls() == envelope


def test_delete_returns_none_on_204():
    seen = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(204)

    admin = Admin("http://x", transport=httpx.MockTransport(handler))

    assert admin.delete_calls() is None


def test_ok_false_in_a_2xx_body_is_not_an_error():
    admin, _ = make_admin({"ok": False, "id": "x"})

    assert admin.app("x") == {"ok": False, "id": "x"}


@pytest.mark.parametrize(
    ("status", "code"),
    [(404, "app_not_found"), (409, "reset_in_progress"), (422, "invalid_manifest")],
)
def test_errors_raise_admin_error(status, code):
    reply = {"error": {"code": code, "message": "nope", "details": [{"pointer": "/a"}]}}
    admin, _ = make_admin(reply, status=status)

    with pytest.raises(AdminError) as err:
        admin.reset()

    assert (err.value.status, err.value.code, err.value.message) == (status, code, "nope")
    assert err.value.details == [{"pointer": "/a"}]
