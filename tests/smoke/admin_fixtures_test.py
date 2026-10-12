"""The admin fixtures work against a live server."""


def test_admin_reaches_health(admin):
    health = admin.health()
    assert health["status"] == "ok"
    assert health["events_dropped"] == 0


def test_faults_fixture_restores_faults_and_delivery_config(faults):
    faults.add_fault("auth.test", {"status": 500, "error": "boom"})
    faults.patch_delivery_config(signature="invalid")
    assert faults.faults() and faults.delivery_config()["signature"] == "invalid"


def test_previous_test_left_no_state_behind(admin):
    assert admin.faults() == []
    assert admin.delivery_config()["signature"] == "valid"
