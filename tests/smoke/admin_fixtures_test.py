"""The admin fixtures work against a live server (control endpoints land stream by stream)."""


def test_admin_reaches_health(admin):
    assert admin.health() == {"status": "ok"}


def test_faults_fixture_teardown_tolerates_missing_endpoints(faults):
    assert faults.base_url
