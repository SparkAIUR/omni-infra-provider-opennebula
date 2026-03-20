from pathlib import Path

from labctl.app import LabApp
from labctl.config import load_config


def test_plan_lists_all_phases() -> None:
    root = Path(__file__).resolve().parents[1]
    app = LabApp(load_config(root / "examples" / "staging-lab.config.example.yaml"))
    plan = app.plan()
    assert [item["name"] for item in plan] == [
        "spot-provision",
        "host-bootstrap",
        "network-overlay",
        "ceph",
        "opennebula",
        "dex",
        "omni",
        "dns-tls",
        "provider",
        "validation",
        "handoff-cluster",
    ]
    assert plan[0]["enabled"] == "no"
    assert plan[-1]["enabled"] == "yes"

