import logging
import runpy
from pathlib import Path

from _pytest.logging import LogCaptureFixture

import infra
from infra.apps import InfraConfig


def test_infra_app_ready_logs(caplog: LogCaptureFixture) -> None:
    caplog.set_level(logging.INFO)
    cfg = InfraConfig("infra", infra)
    cfg.ready()
    assert any("infra_app_ready" in r.message for r in caplog.records)


def test_gunicorn_conf_file_exports_expected_vars() -> None:
    repo_root = Path(__file__).resolve().parents[1]
    conf_path = repo_root / "gunicorn.conf.py"
    assert conf_path.exists()

    ns = runpy.run_path(str(conf_path))
    assert "logconfig_dict" in ns
    assert "accesslog" in ns
