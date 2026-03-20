from pathlib import Path

from labctl.config import load_config


def test_load_example_config() -> None:
    root = Path(__file__).resolve().parents[1]
    config = load_config(root / "examples" / "staging-lab.config.example.yaml")
    assert config.schemaVersion == "v1alpha1"
    assert config.nodes.frontend.hostname == "lab-fe-01"
    assert config.opennebula.template.name == "talos-omni-base"
    assert config.handoffCluster.preferredDatastore == "one-csi-local"

